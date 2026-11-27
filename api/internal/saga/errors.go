package saga

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ErrorGroup é um tipo de erro: mensagens iguais depois de tirar IDs, números, IPs e nomes.
type ErrorGroup struct {
	Key     string `json:"key"`
	Pattern string `json:"pattern"` // mensagem normalizada (o que o grupo tem em comum)
	Example string `json:"example"` // a ocorrência mais recente, completa
	// Published: algum worker publicou ERROR com esta mensagem (é o que derruba a saga)
	Published  bool          `json:"published"`
	Cause      ErrorCause    `json:"cause"`
	Lines      int           `json:"lines"`
	Sagas      int           `json:"sagas"`  // transações distintas
	Failed     int           `json:"failed"` // dessas, quantas falharam (publish ERROR ou ação recusada)
	Apps       []NameCount   `json:"apps"`
	Operations []NameCount   `json:"operations"`
	First      float64       `json:"first"`
	Last       float64       `json:"last"`
	Hist       []int         `json:"hist"` // ocorrências por fatia do período
	Samples    []ErrorSample `json:"samples"`
}

type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ErrorSample: uma transação em que o erro apareceu (a ocorrência mais recente dela).
type ErrorSample struct {
	TransactionID string  `json:"transactionId"`
	TS            float64 `json:"ts"`
	App           string  `json:"app"`
	Operation     string  `json:"operation,omitempty"`
	Org           string  `json:"org,omitempty"`
	Failed        bool    `json:"failed"`
	Msg           string  `json:"msg"`
}

// MaxErrorSamples: transações listadas por grupo.
const MaxErrorSamples = 50

var (
	normTx     = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}(-\d{14})?`)
	normIP     = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}(?::\d+)?\b`)
	normQuoted = regexp.MustCompile("`[^`]*`|'[^']{1,120}'|\\\\?\"[^\"\\\\]*\\\\?\"")
	// nomes gerados (hosts, VMs): palavra longa com dígito e hífen/ponto → <nome>
	normName = regexp.MustCompile(`[A-Za-z0-9_]*\d[A-Za-z0-9_]*(?:[-.][A-Za-z0-9_]+){2,}|[A-Za-z0-9_]+(?:[-.][A-Za-z0-9_]+){2,}[-.][A-Za-z0-9_]*\d[A-Za-z0-9_.-]*`)
	normHex  = regexp.MustCompile(`\b[0-9a-fA-F]{12,}\b`)
	// números soltos viram N; HTTP 500 / status 404 continuam (dizem o tipo do erro)
	normNum   = regexp.MustCompile(`(?i)(HTTP |status[=: ]+|code[=: ]+)?\d+(?:\.\d+)?`)
	normSpace = regexp.MustCompile(`\s+`)
	// "publicou ERROR: x" e "x" logado pelo mesmo worker são o mesmo erro
	normPublished = regexp.MustCompile(`^` + PublishedPrefix + `\S+: `)
	// invólucros do core/v2: "Action execution failed: X" é o mesmo erro que "X"
	normWrapper = regexp.MustCompile(`^(panic recovered: )?(resource action failed: redelivery max attempts exceeded\s+Worker error: )?(Action execution failed: |error publishing message to retry: |Attempt threshold hit: |state after failed action execution: )?`)
)

// NormalizeError reduz a mensagem ao que se repete entre ocorrências do mesmo erro.
func NormalizeError(msg string) string {
	s := normPublished.ReplaceAllString(msg, "")
	s = normWrapper.ReplaceAllString(s, "")
	s = normTx.ReplaceAllString(s, "<id>")
	s = normIP.ReplaceAllString(s, "<ip>")
	s = normQuoted.ReplaceAllStringFunc(s, func(q string) string {
		if strings.HasPrefix(q, "`") {
			return "`…`"
		}
		if strings.HasPrefix(q, "'") {
			return "'…'"
		}
		return `"…"`
	})
	s = normName.ReplaceAllStringFunc(s, func(w string) string {
		if len(w) < 16 {
			return w
		}
		return "<nome>"
	})
	s = normHex.ReplaceAllString(s, "<hex>")
	s = normNum.ReplaceAllStringFunc(s, func(m string) string {
		if sub := normNum.FindStringSubmatch(m); sub[1] != "" {
			return m
		}
		return "N"
	})
	s = strings.TrimSpace(normSpace.ReplaceAllString(s, " "))
	if r := []rune(s); len(r) > 240 {
		s = string(r[:240]) + "…"
	}
	return s
}

// errorOf: a mensagem de erro de uma linha, ou "" se ela não é erro.
// Erro = linha de nível error/fatal/panic, publish com status de falha, ou resposta 4xx/5xx do gateway.
func errorOf(e *Event, info AppInfo) string {
	switch {
	case e.Kind == KindError, e.Kind == KindPublish && PublishFailed(e.Status):
		return eventErrorText(e)
	case e.Kind == KindHTTPOut && info.Role == RoleGateway:
		if n, err := strconv.Atoi(e.Status); err == nil && n >= 400 {
			msg := strings.TrimSpace(e.Msg)
			if m, ok := e.Payload.(map[string]any); ok {
				if v, ok := m["message"].(string); ok && v != "" {
					msg = v
				}
			}
			return "HTTP " + e.Status + ": " + msg
		}
	}
	return ""
}

// ErrorTotals: transações distintas com algum erro, e quantas delas falharam (os grupos se sobrepõem).
type ErrorTotals struct {
	Txs    int `json:"txs"`
	Failed int `json:"failed"`
}

// ErrorGroups agrupa as linhas de erro por tipo. reqs (ação no gateway por transação) dá a operação e
// a organização; failed são as transações em que algum worker publicou ERROR. [from, to] em epoch ms,
// dividido em buckets fatias para o histograma.
func ErrorGroups(lines []RawLine, reqs map[string]Request, failed map[string]bool, from, to float64, buckets int) ([]ErrorGroup, ErrorTotals) {
	return groupErrors(lines, false, "", reqs, failed, from, to, buckets)
}

// ErrorGroupsMatching: como ErrorGroups, contando só as ocorrências cuja mensagem (ou a operação da ação)
// contém q, sem diferença de maiúsculas. Os grupos, as contagens e as sagas listadas são só dessas ocorrências.
func ErrorGroupsMatching(lines []RawLine, q string, reqs map[string]Request, failed map[string]bool, from, to float64, buckets int) ([]ErrorGroup, ErrorTotals) {
	return groupErrors(lines, false, q, reqs, failed, from, to, buckets)
}

// LooseErrorGroups agrupa erros que não têm transação (panics, conexão, fila): sem ações afetadas,
// cada grupo conta linhas e aplicações. q (opcional) mantém só as mensagens que contêm o texto.
func LooseErrorGroups(lines []RawLine, q string, from, to float64, buckets int) []ErrorGroup {
	gs, _ := groupErrors(lines, true, q, nil, nil, from, to, buckets)
	return gs
}

func groupErrors(lines []RawLine, loose bool, q string, reqs map[string]Request, failed map[string]bool, from, to float64, buckets int) ([]ErrorGroup, ErrorTotals) {
	q = strings.ToLower(strings.TrimSpace(q))
	type acc struct {
		g     ErrorGroup
		txs   map[string]*ErrorSample
		apps  map[string]int
		ops   map[string]int
		lastT float64
	}
	byKey := map[string]*acc{}
	span := max(to-from, 1)

	for _, l := range lines {
		e, info, tx, ok := parseLine(l)
		if !ok {
			continue
		}
		msg := errorOf(e, info)
		if msg == "" {
			continue
		}
		if tx == "" {
			tx = txRe.FindString(e.Raw)
		}
		if (tx == "") != loose {
			continue // com transação vai para ErrorGroups, sem vai para LooseErrorGroups
		}
		if q != "" && !strings.Contains(strings.ToLower(msg), q) &&
			(loose || !strings.Contains(strings.ToLower(reqs[tx].Operation), q)) {
			continue
		}
		pat := NormalizeError(msg)
		sum := sha1.Sum([]byte(pat))
		key := hex.EncodeToString(sum[:6])
		a := byKey[key]
		if a == nil {
			a = &acc{
				g:    ErrorGroup{Key: key, Pattern: pat, First: e.TS, Hist: make([]int, buckets)},
				txs:  map[string]*ErrorSample{},
				apps: map[string]int{},
				ops:  map[string]int{},
			}
			byKey[key] = a
		}
		g := &a.g
		g.Lines++
		g.Published = g.Published || strings.HasPrefix(msg, PublishedPrefix)
		g.First = min(g.First, e.TS)
		g.Last = max(g.Last, e.TS)
		if e.TS >= a.lastT {
			a.lastT, g.Example = e.TS, msg
		}
		if i := int((e.TS - from) / span * float64(buckets)); i >= 0 && i < buckets {
			g.Hist[i]++
		}
		a.apps[e.App]++
		if loose {
			continue
		}

		sm := a.txs[tx]
		if sm == nil {
			r, hasReq := reqs[tx]
			sm = &ErrorSample{TransactionID: tx, App: e.App}
			if hasReq {
				sm.Operation, sm.Org = r.Operation, r.Org
				a.ops[r.Operation]++
			}
			// falhou: worker publicou ERROR, ou a ação nem virou saga porque o gateway recusou
			sm.Failed = failed[tx] || (hasReq && !r.Saga && httpFailed(r.Status))
			a.txs[tx] = sm
		}
		if e.TS >= sm.TS {
			sm.TS, sm.App, sm.Msg = e.TS, e.App, msg
		}
	}

	out := make([]ErrorGroup, 0, len(byKey))
	all := map[string]bool{}
	for _, a := range byKey {
		for tx, sm := range a.txs {
			all[tx] = all[tx] || sm.Failed
		}
		g := a.g
		g.Sagas = len(a.txs)
		samples := make([]ErrorSample, 0, len(a.txs))
		for _, sm := range a.txs {
			if sm.Failed {
				g.Failed++
			}
			samples = append(samples, *sm)
		}
		slices.SortFunc(samples, func(x, y ErrorSample) int { return cmpF(y.TS, x.TS) })
		g.Samples = samples[:min(MaxErrorSamples, len(samples))]
		g.Apps, g.Operations = counts(a.apps), counts(a.ops)
		app := ""
		if len(samples) > 0 {
			app = samples[0].App
		} else if len(g.Apps) > 0 {
			app = g.Apps[0].Name
		}
		g.Cause = ClassifyError(g.Example, app)
		out = append(out, g)
	}
	// primeiro o que derruba saga, depois o que afeta mais transações
	slices.SortFunc(out, func(x, y ErrorGroup) int {
		if x.Failed != y.Failed {
			return y.Failed - x.Failed
		}
		if x.Sagas != y.Sagas {
			return y.Sagas - x.Sagas
		}
		return y.Lines - x.Lines
	})
	tot := ErrorTotals{Txs: len(all)}
	for _, f := range all {
		if f {
			tot.Failed++
		}
	}
	return out, tot
}

func httpFailed(status string) bool {
	n, err := strconv.Atoi(status)
	return err == nil && n >= 400
}

func counts(m map[string]int) []NameCount {
	out := make([]NameCount, 0, len(m))
	for k, v := range m {
		out = append(out, NameCount{k, v})
	}
	slices.SortFunc(out, func(a, b NameCount) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// ErrorNews: um tipo de erro da janela recente que é novo (não apareceu na base) ou está em alta.
type ErrorNews struct {
	Kind  string     `json:"kind"` // "new" | "spike"
	Group ErrorGroup `json:"group"`
	// BaseSagas: ações afetadas na base inteira; BasePerWindow: média por janela do mesmo tamanho da recente
	BaseSagas     int     `json:"baseSagas"`
	BasePerWindow float64 `json:"basePerWindow"`
}

// Regras de "em alta": volume mínimo na janela e várias vezes a média da base.
const (
	spikeMinSagas = 5
	spikeRatio    = 4.0
)

// NewsOf compara os grupos da janela recente com os da base (os N dias antes dela). windows = quantas
// janelas do tamanho da recente cabem na base. Novos primeiro, depois em alta; dentro de cada, os que
// fizeram ações falhar e os maiores.
func NewsOf(recent, base []ErrorGroup, windows float64) []ErrorNews {
	baseBy := map[string]ErrorGroup{}
	for _, g := range base {
		baseBy[g.Key] = g
	}
	out := []ErrorNews{}
	for _, g := range recent {
		b, seen := baseBy[g.Key]
		if !seen {
			out = append(out, ErrorNews{Kind: "new", Group: g})
			continue
		}
		per := float64(b.Sagas) / max(windows, 1)
		if g.Sagas >= spikeMinSagas && float64(g.Sagas) >= spikeRatio*per {
			out = append(out, ErrorNews{Kind: "spike", Group: g, BaseSagas: b.Sagas, BasePerWindow: per})
		}
	}
	slices.SortFunc(out, func(x, y ErrorNews) int {
		if x.Kind != y.Kind {
			return strings.Compare(x.Kind, y.Kind) // "new" < "spike"
		}
		if x.Group.Failed != y.Group.Failed {
			return y.Group.Failed - x.Group.Failed
		}
		return y.Group.Sagas - x.Group.Sagas
	})
	return out
}
