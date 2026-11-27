package saga

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reOutboxRelay = regexp.MustCompile(`^(processing message|message handled successfully|message marked as sent)$`)
	reRelayFail   = regexp.MustCompile(`(?i)^message processing failed`)
	reStart       = regexp.MustCompile(`^(Setting up worker|Worker setup)$`)
	reStateIn     = regexp.MustCompile(`^(Preparing to execute action|state before action execution)$`)
	reStepEnd     = regexp.MustCompile(`^(Action executed successfully|state after successful action execution)$`)
	rePublish     = regexp.MustCompile(`^(publishing message|message published( successfully)?)$`)
	reOp          = regexp.MustCompile(`^(\w+?)_(start|end|resolved)$`)
	reErrLevel    = regexp.MustCompile(`(?i)^(error|fatal|panic)$`)
	reExchange    = regexp.MustCompile(`ExchangeName: ([^,]+)`)
	reRouting     = regexp.MustCompile(`RoutingKey: ([^,]+)`)
	reAmqpMsg     = regexp.MustCompile(`Msg: (.*)$`)
)

func ptr(v float64) *float64 { return &v }

func num(s string) *float64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

// classify aplica as regras do catalogo.yaml: primeira regra que casar define o evento.
func classify(app string, role Role, f fields, e *Event) {
	msg := f.str("msg")
	integ := f.str("integration")
	imeth := f.str("integration_method")

	switch role {
	case RoleGateway:
		switch {
		case msg == "request received":
			e.Kind, e.ResourceType = KindHTTPIn, "http"
			e.Target = f.str("method") + " " + f.str("path")
			e.Payload = tryJSON(f["body"])
			return
		case integ != "":
			e.Kind, e.ResourceType, e.Target, e.Payload = KindIntegrationCall, integ, imeth, f["integration_args"]
			return
		case msg == "Relay message added":
			e.Kind, e.ResourceType, e.Target = KindOutboxEnqueue, "outbox", "orchestrator"
			return
		case f.str("status_code") != "":
			e.Kind, e.Status, e.Payload = KindHTTPOut, f.str("status_code"), f["message"]
			return
		case reRelayFail.MatchString(msg):
			// relay do outbox não conseguiu entregar ao orquestrador e vai tentar de novo
			e.Kind, e.ResourceType, e.Target = KindOutboxRelay, "outbox", or(f.str("relay_service"), "orchestrator")
			e.Status, e.Error = "retry", or(f.str("error"), f.str("err"))
			return
		case reOutboxRelay.MatchString(msg):
			e.Kind, e.ResourceType, e.Target = KindOutboxRelay, "outbox", or(f.str("relay_service"), "orchestrator")
			return
		}
	case RoleOrchestrator:
		switch {
		case f.str("package") == "middlewares":
			e.Kind, e.ResourceType = KindJobRequest, "http"
			e.Target = f.str("method") + " " + f.str("path")
			e.Payload = tryJSON(f["method_args"])
			return
		case f.str("package") == "handlers.callbacks":
			// method_resp=POST status_code=202
			parts := strings.FieldsFunc(f.str("method_resp"), func(r rune) bool { return r == ' ' || r == '=' })
			e.Kind = KindJobResponse
			if len(parts) > 0 {
				e.Target = parts[0] + " " + f.str("path")
			}
			if len(parts) > 2 {
				e.Status = parts[2]
			}
			e.DurationMs = num(f.str("elapsed_ms"))
			e.Payload = tryJSON(f["message"])
			return
		case integ == "amqp":
			a := f.str("integration_args")
			e.Kind, e.ResourceType = KindPublish, "queue"
			e.Exchange = sub(reExchange, a)
			e.Routing = sub(reRouting, a)
			e.Target = e.Exchange + " / " + e.Routing
			if m := sub(reAmqpMsg, a); m != "" {
				e.Payload = tryJSON(m)
			}
			return
		}
	}

	// workers: padrões da lib de saga dos workers (valem para qualquer app)
	switch {
	case reStart.MatchString(msg):
		e.Kind = KindStepStart
	case integ == "Log" && f.str("type") == "Init":
		e.Kind, e.Payload = KindStepStart, tryJSON(f["integration_args"])
		e.Action = strings.ToLower(f.str("method")) // core Delete → delete
	case reStateIn.MatchString(msg):
		e.Kind, e.Payload = KindStateIn, tryJSON(f["state"])
	case reStepEnd.MatchString(msg):
		e.Kind, e.Status, e.Payload = KindStepEnd, "ok", tryJSON(f["state"])
	case integ == "Log" && f.str("type") == "Done":
		e.Kind, e.Status, e.Payload = KindStepEnd, "ok", tryJSON(f["integration_args"])
	case rePublish.MatchString(msg):
		e.Kind, e.Status, e.Error = KindPublish, f.str("status"), f.str("status_error_msg")
		if w, ok := f["worker"]; ok {
			e.Payload = tryJSON(w)
		} else {
			e.Payload = tryJSON(f["message"])
		}
	case f.str("method") == "publish" && integ == "johdin":
		e.Kind, e.ResourceType = KindPublish, "queue"
		e.Exchange, e.Routing = f.str("exchange"), f.str("routing")
		e.Target = e.Exchange + " / " + e.Routing
	case reOp.MatchString(msg):
		// logs próprios do worker: <op>_start / <op>_end / <op>_resolved (ex.: fortinet-address)
		m := reOp.FindStringSubmatch(msg)
		vendor := strings.SplitN(app, "-", 2)[0]
		rest := fields{}
		for k, v := range f {
			switch k {
			case "time", "level", "msg", "transaction_id":
			default:
				rest[k] = v
			}
		}
		e.Kind, e.ResourceType = KindIntegrationCall, vendor
		e.Target = m[1] + " (" + m[2] + ")"
		e.Integration, e.IntegrationMethod = vendor, m[1]+"."+m[2]
		e.DurationMs = num(f.str("latency_ms"))
		e.Payload = map[string]any(rest)
	case integ != "" && imeth != "":
		e.Kind, e.ResourceType, e.Target = KindIntegrationCall, integ, imeth
		e.Payload = tryJSON(f["integration_args"])
	case reErrLevel.MatchString(f.str("level")):
		e.Kind, e.Status = KindError, "fail"
	case msg != "":
		e.Kind = KindLog
	default:
		e.Kind = KindUnknown
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
