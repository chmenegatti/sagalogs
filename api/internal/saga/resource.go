package saga

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
)

// ResourceHit é uma transação em que o ID do recurso aparece em algum log.
type ResourceHit struct {
	TransactionID string   `json:"transactionId"`
	First         float64  `json:"first"` // epoch ms da primeira linha que cita o recurso
	Last          float64  `json:"last"`
	Apps          []string `json:"apps"`  // onde o ID aparece (gateway, workers…)
	Lines         int      `json:"lines"` // quantas linhas citam o ID
	InPath        bool     `json:"inPath"`
	Request       *Request `json:"request,omitempty"` // ação no gateway, quando achada
}

var (
	txRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}-\d{14}`)
	// ID aceito na busca: também barra aspas/regex na query do Loki
	ResourceIDRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{4,64}$`)
)

// ResourceHits agrupa por transação as linhas que citam o recurso; gw completa com a ação no gateway.
// Mais recentes primeiro.
func ResourceHits(id string, lines, gw []RawLine) []ResourceHit {
	byTx := map[string]*ResourceHit{}
	for _, l := range lines {
		var d dockerLine
		if err := json.Unmarshal([]byte(l.Line), &d); err != nil || d.Log == "" {
			continue
		}
		tx := txRe.FindString(d.Log)
		if tx == "" {
			continue
		}
		ts, ok := dockerTS(d.Time)
		if !ok {
			ts = float64(l.TS.UnixNano()) / 1e6
		}
		h := byTx[tx]
		if h == nil {
			h = &ResourceHit{TransactionID: tx, First: ts, Last: ts}
			byTx[tx] = h
		}
		h.Lines++
		h.First, h.Last = min(h.First, ts), max(h.Last, ts)
		if tag := strings.Split(d.Attrs.Tag, "|"); len(tag) > 1 && !slices.Contains(h.Apps, tag[1]) {
			h.Apps = append(h.Apps, tag[1])
		}
	}
	for _, r := range Requests(gw) {
		if h := byTx[r.TransactionID]; h != nil {
			h.Request = &r
			h.InPath = strings.Contains(r.Path, id)
		}
	}
	out := make([]ResourceHit, 0, len(byTx))
	for _, h := range byTx {
		out = append(out, *h)
	}
	slices.SortFunc(out, func(a, b ResourceHit) int { return cmpF(b.First, a.First) })
	return out
}

// HitTxs devolve os IDs de transação, na ordem (mais recentes primeiro), até max.
func HitTxs(hits []ResourceHit, max int) []string {
	out := make([]string, 0, min(len(hits), max))
	for _, h := range hits {
		if len(out) == max {
			break
		}
		out = append(out, h.TransactionID)
	}
	return out
}

// AppOf devolve o nome do container de uma linha do export/Loki (attrs.tag "imagem|app|…").
func AppOf(line string) string {
	var d dockerLine
	if json.Unmarshal([]byte(line), &d) != nil {
		return ""
	}
	if tag := strings.Split(d.Attrs.Tag, "|"); len(tag) > 1 {
		return tag[1]
	}
	return ""
}
