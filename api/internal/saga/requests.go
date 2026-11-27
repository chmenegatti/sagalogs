package saga

import (
	"encoding/json"
	"slices"
	"strings"
)

// Request é uma requisição de ação (POST/PUT/PATCH/DELETE) que chegou ao gateway.
type Request struct {
	TransactionID string   `json:"transactionId"`
	TS            float64  `json:"ts"` // epoch ms da entrada
	Method        string   `json:"method"`
	Path          string   `json:"path"`
	Operation     string   `json:"operation"`
	Resource      string   `json:"resource"`
	Org           string   `json:"org,omitempty"`
	User          string   `json:"user,omitempty"`
	Status        string   `json:"status,omitempty"`     // status_code, quando o gateway loga a resposta
	DurationMs    *float64 `json:"durationMs,omitempty"` // entrada → resposta
	Saga          bool     `json:"saga"`                 // gravou no outbox: virou saga nos workers
	SagaFailed    bool     `json:"sagaFailed,omitempty"` // algum worker publicou ERROR
}

var actionMethods = map[string]bool{"POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// Requests agrupa por transaction_id as linhas do gateway e devolve as ações, mais recentes primeiro.
func Requests(lines []RawLine) []Request {
	byTx := map[string]*Request{}
	type extra struct {
		statusTS float64
		status   string
		saga     bool
	}
	extras := map[string]*extra{}

	for _, l := range lines {
		var d dockerLine
		if err := json.Unmarshal([]byte(l.Line), &d); err != nil || d.Log == "" {
			continue
		}
		tag := strings.Split(d.Attrs.Tag, "|")
		if len(tag) < 2 || appInfo(tag[1], "", "").Role != RoleGateway {
			continue
		}
		_, f := parseLog(d.Log)
		tx := f.str("transaction_id")
		if tx == "" {
			continue
		}
		ts, ok := dockerTS(d.Time)
		if !ok {
			ts = float64(l.TS.UnixNano()) / 1e6
		}
		x := extras[tx]
		if x == nil {
			x = &extra{}
			extras[tx] = x
		}
		msg := f.str("msg")
		switch {
		case msg == "request received" && actionMethods[f.str("method")]:
			if r := byTx[tx]; r != nil && r.TS <= ts {
				continue
			}
			path := f.str("path")
			target := f.str("method") + " " + path
			byTx[tx] = &Request{
				TransactionID: tx,
				TS:            ts,
				Method:        f.str("method"),
				Path:          path,
				Operation:     operationName(target),
				Resource:      lastPathSegment(target),
				Org:           pathParam(path, "organizations"),
				User:          f.str("user_id"),
			}
		case f.str("status_code") != "" && actionMethods[firstWord(f.str("method_resp"))]:
			if ts >= x.statusTS {
				x.statusTS, x.status = ts, f.str("status_code")
			}
		case msg == "Relay message added":
			x.saga = true
		}
	}

	out := make([]Request, 0, len(byTx))
	for tx, r := range byTx {
		if x := extras[tx]; x != nil {
			r.Saga = x.saga
			if x.status != "" {
				r.Status = x.status
				d := x.statusTS - r.TS
				if d >= 0 {
					r.DurationMs = ptr(d)
				}
			}
		}
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b Request) int { return cmpF(b.TS, a.TS) })
	return out
}

// pathParam: "/v1/organizations/<id>/nats" + "organizations" → "<id>".
func pathParam(path, key string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == key {
			return parts[i+1]
		}
	}
	return ""
}

func firstWord(s string) string {
	w, _, _ := strings.Cut(strings.TrimSpace(s), " ")
	return w
}
