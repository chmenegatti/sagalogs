package saga

import (
	"math"
	"slices"
	"strings"
)

// CauseKinds: ordem fixa das causas (a mesma das cores na tela).
var CauseKinds = []string{"vendor", "credential", "timeout", "ours", "infra", "concurrency", "retries", "other"}

// CauseSeries: ações que falharam por uma causa, por hora.
type CauseSeries struct {
	Kind    string      `json:"kind"`
	Total   int         `json:"total"`
	Refused int         `json:"refused"`           // dessas, recusadas pelo gateway
	Vendors []NameCount `json:"vendors,omitempty"` // fornecedores envolvidos (vazio = nosso lado)
	Hours   [][3]int    `json:"hours"`             // [índice da hora, ações, dessas recusadas pelo gateway]
}

// FailureCauses: ações que falharam no período (algum worker publicou ERROR, ou o gateway recusou a ação), pela
// causa provável, hora a hora. Mesmo critério de "falhou" da tela Erros.
type FailureCauses struct {
	Start   float64       `json:"start"` // começo da primeira hora (epoch ms)
	NHours  int           `json:"hours"`
	Total   int           `json:"total"`
	Refused int           `json:"refused"` // dessas, recusadas pelo gateway (não viraram saga)
	Kinds   []CauseSeries `json:"kinds"`   // na ordem de CauseKinds, só as que aparecem
}

// FailureCausesOf: cada ação que falhou conta uma vez, na hora da falha (o último publish ERROR, ou a
// última linha de erro), com a causa mais informativa das mensagens dela: a do publish ERROR antes das outras, e
// "retentativas esgotadas" ou "sem classificação" só quando não há outra.
func FailureCausesOf(lines []RawLine, reqs map[string]Request, failed map[string]bool, from, to float64) FailureCauses {
	type txAcc struct {
		cause     ErrorCause
		score     int
		causeTS   float64
		failTS    float64
		published bool
		refused   bool
	}
	type ckey struct{ pat, app string }
	cache := map[ckey]ErrorCause{}
	byTx := map[string]*txAcc{}
	for _, l := range lines {
		e, info, tx, ok := parseLine(l)
		if !ok {
			continue
		}
		if tx == "" {
			tx = txRe.FindString(e.Raw)
		}
		if tx == "" {
			continue
		}
		r, hasReq := reqs[tx]
		refused := hasReq && !r.Saga && httpFailed(r.Status)
		if !failed[tx] && !refused {
			continue
		}
		msg := errorOf(e, info)
		if msg == "" {
			continue
		}
		pub := strings.HasPrefix(msg, PublishedPrefix)
		k := ckey{NormalizeError(msg), e.App}
		c, hit := cache[k]
		if !hit {
			c = ClassifyError(msg, e.App)
			c.Reply = ""
			cache[k] = c
		}
		score := 0
		if c.Kind != "retries" && c.Kind != "other" {
			score += 2
		}
		if pub {
			score++
		}
		a := byTx[tx]
		if a == nil {
			a = &txAcc{score: -1, refused: refused && !failed[tx]}
			byTx[tx] = a
		}
		if score > a.score || (score == a.score && e.TS >= a.causeTS) {
			a.cause, a.score, a.causeTS = c, score, e.TS
		}
		// hora da falha: o último publish ERROR; sem ele, a última linha de erro
		if pub && (!a.published || e.TS >= a.failTS) {
			a.failTS, a.published = e.TS, true
		} else if !a.published && e.TS >= a.failTS {
			a.failTS = e.TS
		}
	}

	start := math.Floor(from/hourMs) * hourMs
	fc := FailureCauses{Start: start, NHours: max(1, int(math.Ceil((to-start)/hourMs))), Kinds: []CauseSeries{}}
	type kindAcc struct {
		total   int
		refused int
		vendors map[string]int
		hours   map[int][2]int
	}
	byKind := map[string]*kindAcc{}
	for _, a := range byTx {
		if a.failTS < from || a.failTS > to {
			continue
		}
		k := byKind[a.cause.Kind]
		if k == nil {
			k = &kindAcc{vendors: map[string]int{}, hours: map[int][2]int{}}
			byKind[a.cause.Kind] = k
		}
		k.total++
		if a.cause.Vendor != "" {
			k.vendors[a.cause.Vendor]++
		}
		h := min(fc.NHours-1, max(0, int((a.failTS-start)/hourMs)))
		v := k.hours[h]
		v[0]++
		fc.Total++
		if a.refused {
			v[1]++
			k.refused++
			fc.Refused++
		}
		k.hours[h] = v
	}
	for _, kind := range CauseKinds {
		k := byKind[kind]
		if k == nil {
			continue
		}
		s := CauseSeries{Kind: kind, Total: k.total, Refused: k.refused, Vendors: counts(k.vendors), Hours: make([][3]int, 0, len(k.hours))}
		for h, v := range k.hours {
			s.Hours = append(s.Hours, [3]int{h, v[0], v[1]})
		}
		slices.SortFunc(s.Hours, func(x, y [3]int) int { return x[0] - y[0] })
		fc.Kinds = append(fc.Kinds, s)
	}
	return fc
}
