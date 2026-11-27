package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sagalogs/internal/saga"
)

// Parsing dos 3 formatos de log suportados:
//   - JSON / slog JSON  (workers com a lib de saga)
//   - logrus texto      (ex.: orquestrador: msg="k=v, k=v, chave[valor]")
//   - slog texto        (ex.: gateway: key=value)

type Kind string

const (
	KindReceived    Kind = "received"    // payload chegou no worker (core: "Preparing to execute action")
	KindDone        Kind = "done"        // ação concluída
	KindFailed      Kind = "failed"      // ação falhou
	KindPublish     Kind = "publish"     // publicou próximo passo
	KindHTTPIn      Kind = "http_in"     // requisição recebida
	KindHTTPOut     Kind = "http_out"    // resposta / chamada HTTP com status
	KindIntegration Kind = "integration" // chamada a sistema externo
	KindError       Kind = "error"
	KindLog         Kind = "log"
)

type Event struct {
	Seq       int64          `json:"seq"`
	TS        time.Time      `json:"ts"`
	Container string         `json:"container"`
	Service   string         `json:"service"`
	Stream    string         `json:"stream"`
	Format    string         `json:"format"`
	Level     string         `json:"level"`
	Kind      Kind           `json:"kind"`
	Msg       string         `json:"msg"`
	Tx        string         `json:"tx,omitempty"`
	Action    string         `json:"action,omitempty"`
	Resource  string         `json:"resource,omitempty"`
	Saga      string         `json:"saga,omitempty"`  // nome da saga na fila/exchange: identity.create
	Step      string         `json:"step,omitempty"`  // etapa: entraid.user.create
	StepN     *int           `json:"stepN,omitempty"` // índice da etapa na saga (o número depois de v1.)
	Target    string         `json:"target,omitempty"`
	Status    string         `json:"status,omitempty"`
	Error     string         `json:"error,omitempty"`
	Payload   any            `json:"payload,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
	Noise     bool           `json:"noise,omitempty"`
	Raw       string         `json:"raw"`
	Image     string         `json:"-"`
}

type fields map[string]any

func (f fields) str(k string) string {
	v, ok := f[k]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}

func parseLog(log string) (format string, f fields) {
	m := strings.TrimSpace(log)
	switch {
	case strings.HasPrefix(m, "{"):
		var j fields
		if err := json.Unmarshal([]byte(m), &j); err != nil {
			return "text", fields{"msg": m}
		}
		if msg, ok := j["msg"].(string); ok {
			if sp := strings.IndexByte(msg, ' '); sp > 0 && wordRe.MatchString(msg[:sp]) && kvRe.MatchString(msg[sp:]) {
				out := fields{}
				for k, v := range parseLogfmt(msg[sp:]) {
					out[k] = v
				}
				for k, v := range j {
					out[k] = v
				}
				out["msg"] = msg[:sp]
				return "json", out
			}
		}
		return "json", j
	case strings.HasPrefix(m, `time="`):
		outer := parseLogfmt(m)
		f := fields{"level": outer["level"]}
		inner := parseCommaKV(outer["msg"])
		for k, v := range inner {
			f[k] = v
		}
		if len(inner) == 0 {
			f["msg"] = outer["msg"]
		}
		return "logrus", f
	case strings.HasPrefix(m, "time=") || logfmtStart.MatchString(m):
		f := fields{}
		for k, v := range parseLogfmt(m) {
			f[k] = v
		}
		return "slog", f
	default:
		return "text", fields{"msg": m}
	}
}

var (
	wordRe      = regexp.MustCompile(`^\w+$`)
	kvRe        = regexp.MustCompile(`\w=`)
	logfmtRe    = regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|\S+)`)
	logfmtStart = regexp.MustCompile(`^\w+=\S`)
	keyRe       = regexp.MustCompile(`^[\w.]+`)
)

func parseLogfmt(s string) map[string]string {
	out := map[string]string{}
	for _, m := range logfmtRe.FindAllStringSubmatch(s, -1) {
		v := m[2]
		if strings.HasPrefix(v, `"`) {
			if u, err := strconv.Unquote(v); err == nil {
				v = u
			} else {
				v = strings.Trim(v, `"`)
			}
		}
		if _, ok := out[m[1]]; !ok {
			out[m[1]] = v
		}
	}
	return out
}

// parseCommaKV lê o msg interno do logrus: "k=v, k=v, chave[valor com {json}], ...".
func parseCommaKV(s string) map[string]string {
	out := map[string]string{}
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == ',') {
			i++
		}
		key := keyRe.FindString(s[i:])
		if key == "" {
			break
		}
		i += len(key)
		if i >= len(s) {
			break
		}
		switch s[i] {
		case '=':
			i++
			end := strings.Index(s[i:], ", ")
			stop := len(s)
			if end >= 0 {
				stop = i + end
			}
			out[key] = s[i:stop]
			i = stop
		case '[':
			depth, inStr, start := 0, false, i+1
		scan:
			for ; i < len(s); i++ {
				c := s[i]
				if inStr {
					if c == '\\' {
						i++
					} else if c == '"' {
						inStr = false
					}
					continue
				}
				switch c {
				case '"':
					inStr = true
				case '[', '{':
					depth++
				case ']', '}':
					depth--
				}
				if depth == 0 && c == ']' {
					break scan
				}
			}
			end := min(i, len(s))
			out[key] = s[start:end]
			i++
		default:
			i++
		}
	}
	return out
}

// tryJSON decodifica strings que parecem JSON (inclusive JSON escapado dentro de string).
func tryJSON(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "{") && !strings.HasPrefix(t, "[") {
		return v
	}
	var out any
	if err := json.Unmarshal([]byte(t), &out); err != nil {
		return v
	}
	return out
}

var (
	reTxRaw    = regexp.MustCompile(`transaction_?[iI][dD]\\*"?\s*[:=]\s*\\*"?([A-Za-z0-9][\w-]{7,})`)
	reStep     = regexp.MustCompile(`\bv\d+\.(\d+)\.([a-z][\w.-]*[a-z0-9])`)
	reStepFlat = regexp.MustCompile(`\bv\d+\.([a-z][\w.-]*[a-z0-9])`)
	reSaga     = regexp.MustCompile(`(?:queue|topic)\.([\w.]+?)\.v\d+\.`)
	reExchange = regexp.MustCompile(`ExchangeName: ([^,]+)`)
	reRouting  = regexp.MustCompile(`RoutingKey: ([^,]+)`)
	reAmqpMsg  = regexp.MustCompile(`Msg: (.*)$`)
	reGin      = regexp.MustCompile(`^\[GIN\] .*\|\s*(\d{3})\s*\|\s*([\d.]+\w+)\s*\|\s*([\d.:a-f]+)\s*\|\s*(\w+)\s+"([^"]+)"`)
)

// noise: healthcheck e afins, escondido por padrão na UI.
var reNoise = regexp.MustCompile(`(?i)health_?check|/healthz|decommission file present`)

// campos que não interessam no painel de detalhes (já viram colunas)
var skipField = map[string]bool{"time": true, "msg": true, "level": true}

func parseLine(container, service, stream string, ts time.Time, raw string) *Event {
	format, f := parseLog(raw)
	e := &Event{
		TS: ts, Container: container, Service: service, Stream: stream, Format: format,
		Raw: raw, Msg: f.str("msg"),
	}
	e.Level = normLevel(f.str("level"), stream, raw)
	if e.Msg == "!EMPTY_MESSAGE" || e.Msg == "[]" {
		e.Msg = ""
	}
	if m := f.str("message"); e.Msg == "" && m != "" && m != "[]" && !strings.HasPrefix(m, "{") {
		e.Msg = m
	}

	classify(container, f, e)
	saga.Observe(service, f) // aprende o papel da app (gateway/orquestrador/worker) pelo que ela loga

	if e.Tx == "" {
		e.Tx = f.str("transaction_id")
	}
	if e.Tx == "" {
		if m := reTxRaw.FindStringSubmatch(raw); m != nil {
			e.Tx = m[1]
		}
	}
	if e.Action == "" {
		e.Action = f.str("action")
	}
	if e.Resource == "" {
		e.Resource = f.str("resource")
	}
	if e.Step == "" {
		for _, k := range []string{"queue", "topic", "routing", "broker_msg_metadata"} {
			if e.setStep(f.str(k)) {
				break
			}
		}
	}
	if e.Kind == "" {
		switch {
		case e.Level == "error":
			e.Kind = KindError
		default:
			e.Kind = KindLog
		}
	}
	if e.Error == "" && (e.Level == "warn" || e.Level == "error") {
		e.Error = or(f.str("error"), f.str("err"))
	}
	e.Noise = reNoise.MatchString(raw)

	if format != "text" {
		e.Fields = map[string]any{}
		for k, v := range f {
			if skipField[k] {
				continue
			}
			e.Fields[k] = tryJSON(v)
		}
		delete(e.Fields, "source") // caminho do arquivo da lib: ruído
	}
	if e.Msg == "" {
		pm := strings.Trim(f.str("package")+"."+f.str("method"), ".")
		e.Msg = strings.TrimSpace(or(e.Target, or(pm, raw)))
	}
	return e
}

func (e *Event) setStep(s string) bool {
	if s == "" {
		return false
	}
	if m := reSaga.FindStringSubmatch(s); m != nil && e.Saga == "" {
		e.Saga = m[1]
	}
	if m := reStep.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		e.StepN, e.Step = &n, m[2]
		return true
	}
	if m := reStepFlat.FindStringSubmatch(s); m != nil {
		e.Step = m[1]
		return true
	}
	return false
}

func normLevel(l, stream, raw string) string {
	switch strings.ToLower(l) {
	case "debug", "trace":
		return "debug"
	case "info", "notice":
		return "info"
	case "warn", "warning":
		return "warn"
	case "error", "err", "fatal", "panic", "critical":
		return "error"
	}
	up := strings.ToUpper(raw)
	switch {
	case strings.Contains(up, "[WARNING]"), strings.HasPrefix(up, "WARN"):
		return "warn"
	case strings.HasPrefix(up, "PANIC"), strings.HasPrefix(up, "ERROR"), strings.HasPrefix(up, "FATAL"):
		return "error"
	}
	return "info"
}

func classify(app string, f fields, e *Event) {
	msg := f.str("msg")
	integ := f.str("integration")
	imeth := f.str("integration_method")

	// padrões da lib core/v2 (todos os workers)
	if integ == "Log" { // core antigo (logrus): integration=Log type=Init|Done
		switch f.str("type") {
		case "Init":
			e.Kind, e.Payload = KindReceived, tryJSON(f["integration_args"])
			e.Action = strings.ToLower(f.str("method"))
			return
		case "Done":
			e.Kind, e.Status, e.Payload = KindDone, "ok", tryJSON(f["integration_args"])
			e.Action = strings.ToLower(f.str("method"))
			return
		}
	}

	switch msg {
	case "Preparing to execute action", "state before action execution":
		e.Kind, e.Payload = KindReceived, tryJSON(f["state"])
		return
	case "Action executed successfully", "state after successful action execution":
		e.Kind, e.Status, e.Payload = KindDone, "ok", tryJSON(f["state"])
		return
	case "Action execution failed":
		e.Kind, e.Status, e.Payload = KindFailed, "fail", tryJSON(f["state"])
		e.Error = or(f.str("err"), f.str("error"))
		return
	case "publishing message":
		e.Kind, e.Payload = KindPublish, tryJSON(f["worker"])
		return
	case "Publishing message", "Publisher success":
		e.Kind, e.Payload = KindPublish, tryJSON(f["message"])
		e.Target = f.str("topic")
		e.setStep(f.str("topic"))
		return
	case "message published successfully", "message published":
		e.Kind, e.Status = KindPublish, "ok"
		return
	case "Publisher error", "publishing to error":
		e.Kind, e.Status, e.Error = KindPublish, "fail", or(f.str("error"), f.str("err"))
		return
	case "request received": // gateway
		e.Kind = KindHTTPIn
		e.Target = strings.TrimSpace(f.str("method") + " " + f.str("path"))
		e.Payload = tryJSON(f["body"])
		return
	}

	switch {
	case f.str("package") == "middlewares": // orquestrador: requisição chegando
		e.Kind = KindHTTPIn
		e.Target = strings.TrimSpace(f.str("method") + " " + f.str("path"))
		e.Payload = tryJSON(f["method_args"])
		e.Msg = or(e.Target, e.Msg)
	case f.str("package") == "handlers.callbacks":
		parts := strings.FieldsFunc(f.str("method_resp"), func(r rune) bool { return r == ' ' || r == '=' })
		e.Kind = KindHTTPOut
		if len(parts) > 0 {
			e.Target = parts[0] + " " + f.str("path")
		}
		if len(parts) > 2 {
			e.Status = parts[2]
		}
		e.Payload = tryJSON(f["message"])
	case integ == "amqp":
		a := f.str("integration_args")
		e.Kind = KindPublish
		ex, rk := sub(reExchange, a), sub(reRouting, a)
		e.Target = strings.Trim(ex+" / "+rk, " /")
		e.setStep(rk)
		if m := sub(reAmqpMsg, a); m != "" {
			e.Payload = tryJSON(m)
		}
	case f.str("method") == "publish" && f.str("routing") != "":
		e.Kind = KindPublish
		e.Target = strings.Trim(f.str("exchange")+" / "+f.str("routing"), " /")
		e.setStep(f.str("exchange") + "." + f.str("routing"))
		e.Payload = tryJSON(f["message"])
	case integ != "":
		e.Kind, e.Target = KindIntegration, strings.Trim(integ+" · "+imeth, " ·")
		e.Payload = tryJSON(f["integration_args"])
	case f.str("status_code") != "":
		e.Kind, e.Status = KindHTTPOut, f.str("status_code")
		e.Target = strings.TrimSpace(f.str("method") + " " + f.str("path"))
	default:
		if m := reGin.FindStringSubmatch(e.Raw); m != nil {
			e.Kind, e.Status, e.Target = KindHTTPOut, m[1], m[4]+" "+m[5]
			e.Msg = m[4] + " " + m[5] + " → " + m[1] + " (" + m[2] + ")"
			if m[1] >= "500" {
				e.Level = "error"
			}
		}
	}
}

func sub(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
