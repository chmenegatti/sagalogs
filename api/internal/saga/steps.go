package saga

import (
	"slices"
	"time"
)

// StepExec: uma execução de worker numa transação (do início à última linha da transação naquele worker).
type StepExec struct {
	App   string
	Start time.Time
	Ms    float64
}

// Deploy: uma imagem nova de um worker entrou no ar.
type Deploy struct {
	Image string  `json:"image"`
	Tag   string  `json:"tag"`
	TS    float64 `json:"ts"` // epoch ms da primeira linha da imagem
}

// StepHour: execuções que começaram numa hora cheia.
type StepHour struct {
	H   float64 `json:"h"` // epoch ms do começo da hora
	N   int     `json:"n"`
	P95 float64 `json:"p95"`
}

// WorkerSteps: tempo das execuções de um worker no período, hora a hora.
type WorkerSteps struct {
	App   string  `json:"app"`
	Label string  `json:"label"`
	N     int     `json:"n"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	// p95 nas últimas 24 h e nos dias antes; Trend = recente / antes - 1 (nil sem execuções suficientes nos dois)
	RecentP95 float64    `json:"recentP95"`
	BeforeP95 float64    `json:"beforeP95"`
	Trend     *float64   `json:"trend"`
	Hours     []StepHour `json:"hours"`
	Deploys   []Deploy   `json:"deploys"`
}

// trendMin: execuções mínimas em cada lado para comparar (com menos, o p95 é só o máximo de um punhado).
const trendMin = 10

// StepSeries agrupa as execuções por worker e hora. recent: começo das "últimas 24 h" da tendência. Ordem: os que
// mais pioraram primeiro, depois os sem tendência pelo p95.
func StepSeries(execs []StepExec, deploys map[string][]Deploy, recent time.Time) []WorkerSteps {
	type acc struct {
		all, rec, old []float64
		hours         map[int64][]float64
	}
	by := map[string]*acc{}
	for _, e := range execs {
		a := by[e.App]
		if a == nil {
			a = &acc{hours: map[int64][]float64{}}
			by[e.App] = a
		}
		a.all = append(a.all, e.Ms)
		if e.Start.Before(recent) {
			a.old = append(a.old, e.Ms)
		} else {
			a.rec = append(a.rec, e.Ms)
		}
		h := e.Start.Truncate(time.Hour).UnixMilli()
		a.hours[h] = append(a.hours[h], e.Ms)
	}
	out := make([]WorkerSteps, 0, len(by))
	for app, a := range by {
		w := WorkerSteps{App: app, Label: appInfo(app, "", "").Label, N: len(a.all), P50: pct(a.all, 50), P95: pct(a.all, 95),
			RecentP95: pct(a.rec, 95), BeforeP95: pct(a.old, 95), Hours: make([]StepHour, 0, len(a.hours)), Deploys: deploys[app]}
		if w.Deploys == nil {
			w.Deploys = []Deploy{}
		}
		if len(a.rec) >= trendMin && len(a.old) >= trendMin && w.BeforeP95 > 0 {
			t := w.RecentP95/w.BeforeP95 - 1
			w.Trend = &t
		}
		for h, v := range a.hours {
			w.Hours = append(w.Hours, StepHour{H: float64(h), N: len(v), P95: pct(v, 95)})
		}
		slices.SortFunc(w.Hours, func(x, y StepHour) int { return int(x.H - y.H) })
		out = append(out, w)
	}
	slices.SortFunc(out, func(x, y WorkerSteps) int {
		switch {
		case x.Trend != nil && y.Trend != nil && *x.Trend != *y.Trend:
			if *x.Trend > *y.Trend {
				return -1
			}
			return 1
		case (x.Trend != nil) != (y.Trend != nil):
			if x.Trend != nil {
				return -1
			}
			return 1
		case x.P95 != y.P95:
			if x.P95 > y.P95 {
				return -1
			}
			return 1
		}
		return 0
	})
	return out
}

// IsWorkerApp: o container é um worker (executa etapas da saga).
func IsWorkerApp(app string) bool { return appInfo(app, "", "").Role == RoleWorker }
