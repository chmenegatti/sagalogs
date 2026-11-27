package saga

import (
	"math"
	"sort"
)

// WorkerMark: um início de worker (com ou sem retentativa) ou um esgotamento de tentativas, tirado do índice local.
type WorkerMark struct {
	App       string
	TS        float64 // epoch ms
	Tx        string
	Start     bool // "Setting up worker"
	Retry     bool // início numa entrega repetida (x-redelivered-amount ≥ 1)
	Exhausted bool // "Attempt threshold hit": desistiu depois da última tentativa
}

// RetryTotals: execuções, retentativas, esgotamentos e sagas que precisaram de retentativa.
type RetryTotals struct {
	Execs     int `json:"execs"`
	Retries   int `json:"retries"`
	Exhausted int `json:"exhausted"`
	Txs       int `json:"txs"`
}

// WorkerRetry: um worker com retentativas no período (ou no período de comparação).
type WorkerRetry struct {
	App string `json:"app"`
	RetryTotals
	Hours [][3]int     `json:"hours"` // [índice da hora, retentativas, esgotamentos]
	Prev  *RetryTotals `json:"prev,omitempty"`
}

// RetryMetrics: retentativas por worker e hora, com o período de comparação (o mesmo horário do dia anterior).
type RetryMetrics struct {
	Start    float64       `json:"start"` // começo da primeira hora (epoch ms)
	NHours   int           `json:"hours"`
	Totals   RetryTotals   `json:"totals"`
	Prev     *RetryTotals  `json:"prev,omitempty"`
	PrevFrom float64       `json:"prevFrom,omitempty"`
	PrevTo   float64       `json:"prevTo,omitempty"`
	Workers  []WorkerRetry `json:"workers"`
}

type retryAcc struct {
	t     RetryTotals
	txs   map[string]bool
	hours map[int][2]int
}

func countRetries(marks []WorkerMark, hourOf func(float64) int) (map[string]*retryAcc, RetryTotals) {
	by := map[string]*retryAcc{}
	var all RetryTotals
	allTxs := map[string]bool{}
	for _, m := range marks {
		a := by[m.App]
		if a == nil {
			a = &retryAcc{txs: map[string]bool{}, hours: map[int][2]int{}}
			by[m.App] = a
		}
		if m.Start {
			a.t.Execs++
			all.Execs++
		}
		if m.Retry {
			a.t.Retries++
			all.Retries++
			if m.Tx != "" {
				a.txs[m.Tx] = true
				allTxs[m.Tx] = true
			}
		}
		if m.Exhausted {
			a.t.Exhausted++
			all.Exhausted++
		}
		if hourOf != nil && (m.Retry || m.Exhausted) {
			i := hourOf(m.TS)
			v := a.hours[i]
			if m.Retry {
				v[0]++
			}
			if m.Exhausted {
				v[1]++
			}
			a.hours[i] = v
		}
	}
	for _, a := range by {
		a.t.Txs = len(a.txs)
	}
	all.Txs = len(allTxs)
	return by, all
}

// RetryMetricsOf junta as marcas do período [from, to] e, com compare, as de [prevFrom, prevTo]. Só entram os
// workers com retentativa ou esgotamento em algum dos dois. Ordem: mais retentativas, depois mais esgotamentos.
// Retentativa sobe antes da falha: o worker passa a precisar de várias entregas para concluir.
func RetryMetricsOf(cur []WorkerMark, from, to float64, prev []WorkerMark, prevFrom, prevTo float64, compare bool) RetryMetrics {
	start := math.Floor(from/hourMs) * hourMs
	rm := RetryMetrics{Start: start, NHours: max(1, int(math.Ceil((to-start)/hourMs))), Workers: []WorkerRetry{}}
	hourOf := func(ms float64) int { return min(rm.NHours-1, max(0, int((ms-start)/hourMs))) }
	now, totals := countRetries(cur, hourOf)
	rm.Totals = totals
	var before map[string]*retryAcc
	if compare {
		var pt RetryTotals
		before, pt = countRetries(prev, nil)
		rm.Prev, rm.PrevFrom, rm.PrevTo = &pt, prevFrom, prevTo
	}
	apps := map[string]bool{}
	for app, a := range now {
		if a.t.Retries+a.t.Exhausted > 0 {
			apps[app] = true
		}
	}
	for app, a := range before {
		if a.t.Retries+a.t.Exhausted > 0 {
			apps[app] = true
		}
	}
	for app := range apps {
		w := WorkerRetry{App: app, Hours: [][3]int{}}
		if a := now[app]; a != nil {
			w.RetryTotals = a.t
			for i, v := range a.hours {
				w.Hours = append(w.Hours, [3]int{i, v[0], v[1]})
			}
			sort.Slice(w.Hours, func(i, j int) bool { return w.Hours[i][0] < w.Hours[j][0] })
		}
		if compare {
			p := RetryTotals{}
			if a := before[app]; a != nil {
				p = a.t
			}
			w.Prev = &p
		}
		rm.Workers = append(rm.Workers, w)
	}
	sort.Slice(rm.Workers, func(i, j int) bool {
		a, b := rm.Workers[i], rm.Workers[j]
		if a.Retries != b.Retries {
			return a.Retries > b.Retries
		}
		if a.Exhausted != b.Exhausted {
			return a.Exhausted > b.Exhausted
		}
		return a.App < b.App
	})
	return rm
}
