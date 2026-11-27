package saga

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// HeatRow: uma operação no mapa de falhas. Cells só traz as horas com alguma ação:
// [hora desde o começo do mapa, ações, recusadas pelo gateway (HTTP ≥ 400), sagas que falharam].
type HeatRow struct {
	Operation string   `json:"operation"`
	Total     int      `json:"total"`
	Failed    int      `json:"failed"` // recusadas + sagas que falharam (uma ação conta uma vez)
	Cells     [][4]int `json:"cells"`
}

// Heatmap: ações por operação e hora. Start é o começo da primeira hora (epoch ms); Hours, quantas colunas.
type Heatmap struct {
	Start float64   `json:"start"`
	Hours int       `json:"hours"`
	Rows  []HeatRow `json:"rows"`
}

const hourMs = 3600e3

// Refused: o gateway respondeu 4xx/5xx.
func Refused(q Request) bool {
	n, err := strconv.Atoi(q.Status)
	return err == nil && n >= 400
}

// HeatmapOf conta as ações de [from, to] por operação e hora (horas cheias; o fuso de São Paulo não tem fração de
// hora, então a hora UTC é a hora local). Linhas: as que mais falharam primeiro, depois as mais usadas.
func HeatmapOf(reqs []Request, from, to float64) Heatmap {
	start := math.Floor(from/hourMs) * hourMs
	hm := Heatmap{Start: start, Hours: max(1, int(math.Ceil((to-start)/hourMs))), Rows: []HeatRow{}}
	type acc struct {
		row   HeatRow
		cells map[int]*[4]int
	}
	by := map[string]*acc{}
	for _, q := range reqs {
		if q.TS < from || q.TS > to {
			continue
		}
		op := q.Operation
		if op == "" {
			op = q.Method + " " + q.Path
		}
		a := by[op]
		if a == nil {
			a = &acc{row: HeatRow{Operation: op}, cells: map[int]*[4]int{}}
			by[op] = a
		}
		h := min(hm.Hours-1, int((q.TS-start)/hourMs))
		c := a.cells[h]
		if c == nil {
			c = &[4]int{h}
			a.cells[h] = c
		}
		c[1]++
		a.row.Total++
		refused, failed := Refused(q), q.SagaFailed
		if refused {
			c[2]++
		}
		if failed {
			c[3]++
		}
		if refused || failed {
			a.row.Failed++
		}
	}
	for _, a := range by {
		for _, c := range a.cells {
			a.row.Cells = append(a.row.Cells, *c)
		}
		slices.SortFunc(a.row.Cells, func(x, y [4]int) int { return x[0] - y[0] })
		hm.Rows = append(hm.Rows, a.row)
	}
	slices.SortFunc(hm.Rows, func(a, b HeatRow) int {
		if a.Failed != b.Failed {
			return b.Failed - a.Failed
		}
		if a.Total != b.Total {
			return b.Total - a.Total
		}
		return strings.Compare(a.Operation, b.Operation)
	})
	return hm
}
