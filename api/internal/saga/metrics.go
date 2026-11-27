package saga

import (
	"math"
	"slices"
	"strings"
)

// OpKey: método e recurso de uma ação (o que dá nome à operação: POST + nats = "Criar NAT").
type OpKey struct {
	Method string
	Res    string
}

// OpKeyOf: chave da ação a partir do método e do caminho (o recurso é o último segmento que não é ID).
func OpKeyOf(method, path string) OpKey {
	return OpKey{Method: strings.ToUpper(method), Res: lastPathSegment(path)}
}

// Operation: nome da operação ("Criar NAT").
func (k OpKey) Operation() string { return operationName(k.Method + " /" + k.Res) }

// ActionCount: ações de uma (método, recurso): quantas chegaram, quantas o gateway recusou (HTTP ≥ 400) e quantas
// viraram saga.
type ActionCount struct {
	Total   int
	Refused int
	Saga    int
}

// ActionCounts: contagens por (método, recurso) num período.
type ActionCounts map[OpKey]ActionCount

// ActionHours: contagens por (método, recurso) e hora; a chave interna é o começo da hora em epoch ms.
// Saga não é preenchido.
type ActionHours map[OpKey]map[int64]ActionCount

// FailedSaga: uma saga em que algum worker publicou ERROR, com a ação de origem e quando ela chegou (epoch ms).
type FailedSaga struct {
	Key OpKey
	TS  float64
}

// CountActions conta as ações de [from, to] (epoch ms) por (método, recurso). Usado pela fonte em arquivo e
// nos testes; no Loki a contagem é feita por ele (consulta de métrica).
func CountActions(reqs []Request, from, to float64) ActionCounts {
	out := ActionCounts{}
	for _, q := range reqs {
		if q.TS < from || q.TS > to {
			continue
		}
		k := OpKeyOf(q.Method, q.Path)
		c := out[k]
		c.Total++
		if Refused(q) {
			c.Refused++
		}
		if q.Saga {
			c.Saga++
		}
		out[k] = c
	}
	return out
}

// HourActions conta as ações de [from, to] por (método, recurso) e hora cheia.
func HourActions(reqs []Request, from, to float64) ActionHours {
	out := ActionHours{}
	for _, q := range reqs {
		if q.TS < from || q.TS > to {
			continue
		}
		k := OpKeyOf(q.Method, q.Path)
		if out[k] == nil {
			out[k] = map[int64]ActionCount{}
		}
		h := int64(math.Floor(q.TS/hourMs) * hourMs)
		c := out[k][h]
		c.Total++
		if Refused(q) {
			c.Refused++
		}
		out[k][h] = c
	}
	return out
}

// OpTotals: soma de ações, falhas e sagas.
type OpTotals struct {
	Total  int `json:"total"`
	Failed int `json:"failed"`
	Saga   int `json:"saga"`
}

// OpMetric: uma operação no período.
type OpMetric struct {
	Operation string `json:"operation"`
	Total     int    `json:"total"`
	Refused   int    `json:"refused"`    // recusadas pelo gateway (HTTP ≥ 400)
	Failed    int    `json:"failed"`     // recusadas + sagas que falharam
	SagaFail  int    `json:"sagaFailed"` // sagas em que um worker publicou ERROR
	Saga      int    `json:"saga"`       // viraram saga
	// Prev: o mesmo período no dia anterior (só quando o período tem até 24 h).
	Prev *OpTotals `json:"prev,omitempty"`
	// Hours: [hora desde Start, ações, falhas], só as horas com ação.
	Hours [][3]int `json:"hours"`
}

// OpsMetrics: taxa de sucesso por operação num período.
type OpsMetrics struct {
	Start    float64    `json:"start"` // começo da primeira hora (epoch ms)
	NHours   int        `json:"hours"`
	Totals   OpTotals   `json:"totals"`
	Prev     *OpTotals  `json:"prev,omitempty"`
	PrevFrom float64    `json:"prevFrom,omitempty"`
	PrevTo   float64    `json:"prevTo,omitempty"`
	Ops      []OpMetric `json:"ops"`
}

// OpsMetricsOf junta as contagens do período (cur), as por hora, as sagas que falharam e, se houver, as do período
// de comparação (prev, em [prevFrom, prevTo]). Operações com o mesmo nome (PUT e PATCH = "Editar") viram uma só.
// Ordem: as que mais falharam, depois as mais usadas.
func OpsMetricsOf(cur ActionCounts, hours ActionHours, failed []FailedSaga, from, to float64,
	prev ActionCounts, prevFrom, prevTo float64) OpsMetrics {
	start := math.Floor(from/hourMs) * hourMs
	om := OpsMetrics{Start: start, NHours: max(1, int(math.Ceil((to-start)/hourMs))), Ops: []OpMetric{}}

	type acc struct {
		m     OpMetric
		prev  *OpTotals
		hours map[int][2]int
	}
	by := map[string]*acc{}
	get := func(k OpKey) *acc {
		if k.Res == "" {
			return nil
		}
		name := k.Operation()
		a := by[name]
		if a == nil {
			a = &acc{m: OpMetric{Operation: name}, hours: map[int][2]int{}}
			by[name] = a
		}
		return a
	}
	hourOf := func(ms float64) int { return min(om.NHours-1, max(0, int((ms-start)/hourMs))) }

	for k, c := range cur {
		if a := get(k); a != nil {
			a.m.Total += c.Total
			a.m.Refused += c.Refused
			a.m.Saga += c.Saga
		}
	}
	for k, hs := range hours {
		a := get(k)
		if a == nil {
			continue
		}
		for h, c := range hs {
			if float64(h) < start || float64(h) > to {
				continue
			}
			i := hourOf(float64(h))
			v := a.hours[i]
			v[0] += c.Total
			v[1] += c.Refused
			a.hours[i] = v
		}
	}
	var prevTot *OpTotals
	if prev != nil {
		prevTot = &OpTotals{}
		om.PrevFrom, om.PrevTo = prevFrom, prevTo
		for k, c := range prev {
			a := get(k)
			if a == nil {
				continue
			}
			if a.prev == nil {
				a.prev = &OpTotals{}
			}
			a.prev.Total += c.Total
			a.prev.Failed += c.Refused
			a.prev.Saga += c.Saga
		}
	}
	for _, f := range failed {
		a := get(f.Key)
		if a == nil {
			continue
		}
		switch {
		case f.TS >= from && f.TS <= to:
			a.m.SagaFail++
			i := hourOf(f.TS)
			v := a.hours[i]
			v[1]++
			a.hours[i] = v
		case prev != nil && f.TS >= prevFrom && f.TS <= prevTo:
			if a.prev == nil {
				a.prev = &OpTotals{}
			}
			a.prev.Failed++
		}
	}

	for _, a := range by {
		m := a.m
		m.Failed = min(m.Total, m.Refused+m.SagaFail)
		if m.Total == 0 && m.SagaFail == 0 && a.prev == nil {
			continue
		}
		if prev != nil {
			m.Prev = a.prev
			if m.Prev == nil {
				m.Prev = &OpTotals{}
			}
			m.Prev.Failed = min(m.Prev.Total, m.Prev.Failed)
			prevTot.Total += m.Prev.Total
			prevTot.Failed += m.Prev.Failed
			prevTot.Saga += m.Prev.Saga
		}
		m.Hours = [][3]int{}
		for i, v := range a.hours {
			m.Hours = append(m.Hours, [3]int{i, v[0], min(v[0], v[1])})
		}
		slices.SortFunc(m.Hours, func(x, y [3]int) int { return x[0] - y[0] })
		om.Totals.Total += m.Total
		om.Totals.Failed += m.Failed
		om.Totals.Saga += m.Saga
		om.Ops = append(om.Ops, m)
	}
	om.Prev = prevTot
	slices.SortFunc(om.Ops, func(x, y OpMetric) int {
		if x.Failed != y.Failed {
			return y.Failed - x.Failed
		}
		if x.Total != y.Total {
			return y.Total - x.Total
		}
		return strings.Compare(x.Operation, y.Operation)
	})
	return om
}

// SagaDur: uma saga concluída, a ação de origem e quanto levou do gateway ao worker final (ms).
type SagaDur struct {
	Key OpKey
	Ms  float64
}

// OpDuration: tempo total das sagas concluídas de uma operação, do gateway ao worker final (ms).
type OpDuration struct {
	Operation string  `json:"operation,omitempty"`
	N         int     `json:"n"` // sagas medidas
	P50       float64 `json:"p50"`
	P95       float64 `json:"p95"`
	P99       float64 `json:"p99"`
	Max       float64 `json:"max"`
	// Hist: sagas por faixa de DurBuckets (a última sem teto), para somar dias e tirar percentis de um mês.
	Hist []int `json:"hist,omitempty"`
}

// DurBuckets: tetos das faixas do histograma do tempo das sagas, em ms.
var DurBuckets = []float64{1e3, 2e3, 3e3, 5e3, 7e3, 10e3, 15e3, 20e3, 30e3, 45e3, 60e3, 90e3, 120e3, 180e3, 300e3, 600e3, 900e3, 1800e3, 3600e3}

func durHist(v []float64) []int {
	h := make([]int, len(DurBuckets)+1)
	for _, x := range v {
		i, _ := slices.BinarySearch(DurBuckets, x)
		h[i]++
	}
	return h
}

// HistPct: percentil p (0–100) aproximado de um histograma de DurBuckets, interpolando dentro da faixa.
func HistPct(h []int, p float64) float64 {
	n := 0
	for _, c := range h {
		n += c
	}
	if n == 0 {
		return 0
	}
	target := p / 100 * float64(n)
	acc := 0.0
	for i, c := range h {
		if c == 0 {
			continue
		}
		if acc+float64(c) >= target {
			lo := 0.0
			if i > 0 {
				lo = DurBuckets[i-1]
			}
			hi := lo * 2
			if i < len(DurBuckets) {
				hi = DurBuckets[i]
			}
			return lo + (hi-lo)*(target-acc)/float64(c)
		}
		acc += float64(c)
	}
	return DurBuckets[len(DurBuckets)-1]
}

// OpsDurations: tempo total das sagas, somando todas as operações (All) e por operação.
type OpsDurations struct {
	All OpDuration   `json:"all"`
	Ops []OpDuration `json:"ops"`
}

// DurationsOf calcula p50/p95/p99 por operação (as com o mesmo nome viram uma só). Ordem: maior p95 primeiro.
func DurationsOf(ds []SagaDur) OpsDurations {
	by := map[string][]float64{}
	var all []float64
	for _, d := range ds {
		if d.Key.Res == "" {
			continue
		}
		name := d.Key.Operation()
		by[name] = append(by[name], d.Ms)
		all = append(all, d.Ms)
	}
	of := func(name string, v []float64) OpDuration {
		slices.Sort(v)
		return OpDuration{Operation: name, N: len(v), P50: pct(v, 50), P95: pct(v, 95), P99: pct(v, 99), Max: pct(v, 100), Hist: durHist(v)}
	}
	out := OpsDurations{All: of("", all), Ops: []OpDuration{}}
	for name, v := range by {
		out.Ops = append(out.Ops, of(name, v))
	}
	slices.SortFunc(out.Ops, func(x, y OpDuration) int {
		if x.P95 != y.P95 {
			return cmpF(y.P95, x.P95)
		}
		return strings.Compare(x.Operation, y.Operation)
	})
	return out
}
