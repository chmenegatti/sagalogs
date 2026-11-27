package saga

import (
	"math"
	"slices"
	"time"
)

// StepPerf é o desempenho de uma etapa (um worker, somando execuções e polling; ou gateway/outbox/orquestrador)
// numa amostra de sagas da mesma operação.
type StepPerf struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	App      string    `json:"app"`
	Kind     StepKind  `json:"kind"`
	Order    float64   `json:"order"` // posição média na saga (para ordenar)
	N        int       `json:"n"`     // sagas da amostra que passaram por ela
	P50      float64   `json:"p50"`   // duração (ms)
	P95      float64   `json:"p95"`
	Max      float64   `json:"max"`
	WaitP50  float64   `json:"waitP50"` // parada antes da etapa (fila, espera do job)
	WaitP95  float64   `json:"waitP95"`
	Fails    int       `json:"fails"` // publicou ERROR
	Warns    int       `json:"warns"` // logou erro sem publicar
	Slowest  []SlowRef `json:"slowest"`
	Attempts float64   `json:"attempts"` // execuções médias (polling)
}

type SlowRef struct {
	TransactionID string  `json:"transactionId"`
	Ms            float64 `json:"ms"`
	Wait          float64 `json:"waitMs"`
	Start         float64 `json:"start"`
}

type DayPerf struct {
	Day   string  `json:"day"` // AAAA-MM-DD, horário de Brasília
	N     int     `json:"n"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	Fails int     `json:"fails"`
}

type PerfReport struct {
	Operation string     `json:"operation"`
	N         int        `json:"n"`
	P50       float64    `json:"p50"` // ponta a ponta
	P95       float64    `json:"p95"`
	Max       float64    `json:"max"`
	Fails     int        `json:"fails"`
	Steps     []StepPerf `json:"steps"`
	Days      []DayPerf  `json:"days"`
}

func stepKey(st *Step) (key, label string) {
	switch st.Kind {
	case StepWorker:
		return "w:" + st.App, st.App
	case StepGateway:
		return "gateway", "Requisição recebida"
	case StepOutbox:
		return "outbox", "Outbox → orquestrador"
	default:
		return string(st.Kind), "Job criado"
	}
}

// Perf agrega as sagas (da mesma operação) por etapa. Durações em ms.
func Perf(op string, sagas []*Saga) PerfReport {
	rep := PerfReport{Operation: op, N: len(sagas)}
	type acc struct {
		sp          StepPerf
		durs, waits []float64
		orders      []float64
		execs       int
		slow        []SlowRef
	}
	byKey := map[string]*acc{}
	var totals []float64
	days := map[string][]*Saga{}

	for _, s := range sagas {
		totals = append(totals, s.DurationMs)
		if s.Status == "fail" {
			rep.Fails++
		}
		day := time.UnixMilli(int64(s.Start)).In(brt).Format("2006-01-02")
		days[day] = append(days[day], s)

		// por saga: soma das execuções da mesma etapa e da parada antes de cada uma
		type per struct {
			dur, wait float64
			first     int
			execs     int
			fail      bool
			warn      bool
			label     string
			app       string
			kind      StepKind
		}
		cur := map[string]*per{}
		var keys []string
		for i, st := range s.Steps {
			k, label := stepKey(st)
			p := cur[k]
			if p == nil {
				p = &per{first: i, label: label, app: st.App, kind: st.Kind}
				cur[k] = p
				keys = append(keys, k)
			}
			p.dur += st.DurationMs
			if i > 0 && i-1 < len(s.Edges) {
				p.wait += s.Edges[i-1].WaitMs
			}
			p.execs += max(1, st.Attempts)
			p.fail = p.fail || st.Status == "fail"
			p.warn = p.warn || st.Status == "warn"
		}
		for _, k := range keys {
			p := cur[k]
			a := byKey[k]
			if a == nil {
				a = &acc{sp: StepPerf{Key: k, Label: p.label, App: p.app, Kind: p.kind}}
				byKey[k] = a
			}
			a.durs = append(a.durs, p.dur)
			a.waits = append(a.waits, p.wait)
			a.orders = append(a.orders, float64(p.first))
			a.execs += p.execs
			if p.fail {
				a.sp.Fails++
			}
			if p.warn {
				a.sp.Warns++
			}
			a.slow = append(a.slow, SlowRef{TransactionID: s.TransactionID, Ms: p.dur, Wait: p.wait, Start: s.Start})
		}
	}

	rep.P50, rep.P95, rep.Max = pct(totals, 50), pct(totals, 95), pct(totals, 100)
	for _, a := range byKey {
		sp := a.sp
		sp.N = len(a.durs)
		sp.P50, sp.P95, sp.Max = pct(a.durs, 50), pct(a.durs, 95), pct(a.durs, 100)
		sp.WaitP50, sp.WaitP95 = pct(a.waits, 50), pct(a.waits, 95)
		sp.Order = mean(a.orders)
		sp.Attempts = math.Round(float64(a.execs)/float64(sp.N)*10) / 10
		slices.SortFunc(a.slow, func(x, y SlowRef) int { return cmpF(y.Ms+y.Wait, x.Ms+x.Wait) })
		sp.Slowest = a.slow[:min(5, len(a.slow))]
		rep.Steps = append(rep.Steps, sp)
	}
	slices.SortFunc(rep.Steps, func(x, y StepPerf) int { return cmpF(x.Order, y.Order) })

	for day, ss := range days {
		var ds []float64
		d := DayPerf{Day: day, N: len(ss)}
		for _, s := range ss {
			ds = append(ds, s.DurationMs)
			if s.Status == "fail" {
				d.Fails++
			}
		}
		d.P50, d.P95 = pct(ds, 50), pct(ds, 95)
		rep.Days = append(rep.Days, d)
	}
	slices.SortFunc(rep.Days, func(x, y DayPerf) int {
		if x.Day < y.Day {
			return -1
		}
		return 1
	})
	return rep
}

// pct: percentil por interpolação linear (p=100 é o máximo).
func pct(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	r := p / 100 * float64(len(s)-1)
	lo := int(math.Floor(r))
	hi := int(math.Ceil(r))
	return math.Round(s[lo] + (s[hi]-s[lo])*(r-float64(lo)))
}

func mean(v []float64) float64 {
	t := 0.0
	for _, x := range v {
		t += x
	}
	return t / float64(max(1, len(v)))
}

// SampleEven escolhe até n itens espalhados uniformemente (a lista vem em ordem de tempo).
func SampleEven[T any](v []T, n int) []T {
	if len(v) <= n {
		return v
	}
	out := make([]T, 0, n)
	step := float64(len(v)) / float64(n)
	for i := range n {
		out = append(out, v[int(float64(i)*step)])
	}
	return out
}
