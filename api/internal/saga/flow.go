package saga

import "math"

// FlowMetrics: sagas abertas (sem worker final) no fim de cada hora, separadas em andamento (alguma linha nos
// últimos IdleMs), paradas (sem linha há IdleMs ou mais) e paradas com falha (algum worker publicou ERROR ou
// esgotou as tentativas). Fila crescendo aparece como andamento subindo; worker travado, como paradas subindo.
type FlowMetrics struct {
	Start      float64  `json:"start"` // começo da primeira hora (epoch ms)
	NHours     int      `json:"hours"`
	IdleMs     float64  `json:"idleMs"`
	LookbackMs float64  `json:"lookbackMs"`
	Points     [][4]int `json:"points"` // [índice da hora, andamento, paradas, paradas com falha], no fim da hora (ou agora)
	Sagas      int      `json:"sagas"`  // sagas iniciadas no período
	Finished   int      `json:"finished"`
	Peak       FlowPeak `json:"peak"`
	Now        *[3]int  `json:"now,omitempty"` // andamento, paradas, com falha agora (quando o período chega até agora)
}

// FlowPeak: hora com mais sagas abertas (andamento + paradas + com falha).
type FlowPeak struct {
	Hour int `json:"hour"`
	Open int `json:"open"`
}

type flowTx struct {
	start, last float64
	failed      bool
}

// FlowTracker conta as sagas abertas varrendo as linhas em ordem de tempo. Uma saga entra quando chega ao gateway
// e sai quando aparece num worker final; aberta há mais de lookback, deixa de contar (abandonada).
type FlowTracker struct {
	m        FlowMetrics
	from, to float64
	now      float64
	idle     float64
	lookback float64
	starts   []Request // ordem de entrada
	next     int       // próxima de starts a abrir
	open     map[string]*flowTx
	ended    map[string]bool
	hour     int     // próxima hora a fechar
	covered  float64 // horas que fecham antes disso ficam de fora (sem dados)
}

// CoveredFrom: as horas que fecham antes de ms (epoch) não têm dados e não entram na curva.
func (f *FlowTracker) CoveredFrom(ms float64) { f.covered = ms }

// NewFlowTracker: reqs com as ações de [from-lookback, to]; só as que viraram saga contam.
func NewFlowTracker(reqs []Request, from, to, now, idle, lookback float64) *FlowTracker {
	start := math.Floor(from/hourMs) * hourMs
	f := &FlowTracker{
		m:    FlowMetrics{Start: start, NHours: max(1, int(math.Ceil((to-start)/hourMs))), IdleMs: idle, LookbackMs: lookback, Points: [][4]int{}},
		from: from, to: to, now: now, idle: idle, lookback: lookback,
		open: map[string]*flowTx{}, ended: map[string]bool{},
	}
	for _, q := range reqs {
		if q.Saga && q.TS <= to {
			f.starts = append(f.starts, q)
		}
	}
	// Requests vem mais recente primeiro
	for i, j := 0, len(f.starts)-1; i < j; i, j = i+1, j-1 {
		f.starts[i], f.starts[j] = f.starts[j], f.starts[i]
	}
	for _, q := range f.starts {
		if q.TS >= from {
			f.m.Sagas++
		}
	}
	return f
}

// Event: uma linha da transação tx em ts (ms), em ordem de tempo. final: num worker final; failed: publicou ERROR
// ou esgotou as tentativas.
func (f *FlowTracker) Event(tx string, ts float64, final, failed bool) {
	f.flush(ts)
	f.admit(ts)
	s := f.open[tx]
	if s == nil {
		return
	}
	if final {
		delete(f.open, tx)
		f.ended[tx] = true
		if s.start >= f.from {
			f.m.Finished++
		}
		return
	}
	s.last = max(s.last, ts)
	s.failed = s.failed || failed
}

// admit abre as sagas que chegaram ao gateway até ts.
func (f *FlowTracker) admit(ts float64) {
	for f.next < len(f.starts) && f.starts[f.next].TS <= ts {
		q := f.starts[f.next]
		f.next++
		if !f.ended[q.TransactionID] && f.open[q.TransactionID] == nil {
			f.open[q.TransactionID] = &flowTx{start: q.TS, last: q.TS}
		}
	}
}

// flush fecha as horas que terminam antes de ts.
func (f *FlowTracker) flush(ts float64) {
	for f.hour < f.m.NHours {
		at := min(f.m.Start+float64(f.hour+1)*hourMs, f.to, f.now)
		if at > ts {
			return
		}
		f.admit(at)
		f.sample(at)
	}
}

func (f *FlowTracker) sample(at float64) {
	h := f.hour
	f.hour++
	if f.m.Start+float64(h)*hourMs > f.now || at < f.covered {
		return // hora ainda não começou, ou antes do banco
	}
	var p [4]int
	p[0] = h
	for tx, s := range f.open {
		if s.start < at-f.lookback {
			delete(f.open, tx)
			continue
		}
		switch {
		case at-s.last < f.idle:
			p[1]++
		case s.failed:
			p[3]++
		default:
			p[2]++
		}
	}
	f.m.Points = append(f.m.Points, p)
	if o := p[1] + p[2] + p[3]; o > f.m.Peak.Open {
		f.m.Peak = FlowPeak{Hour: h, Open: o}
	}
	if at == f.now {
		n := [3]int{p[1], p[2], p[3]}
		f.m.Now = &n
	}
}

// Done fecha as horas que faltam.
func (f *FlowTracker) Done() FlowMetrics {
	f.flush(math.Inf(1))
	return f.m
}
