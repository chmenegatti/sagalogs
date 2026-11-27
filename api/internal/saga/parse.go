package saga

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RawLine é uma linha como o Loki guarda: o JSON do driver json-file do Docker.
//
//	{"log":"...","stream":"stderr","attrs":{"tag":"imagem:versao|app|sha|container"},"time":"2026-...Z"}
type RawLine struct {
	Line string
	// TS é o timestamp do Loki; usado só se o JSON do Docker não trouxer "time".
	TS time.Time
}

type dockerLine struct {
	Log   string `json:"log"`
	Time  string `json:"time"`
	Attrs struct {
		Tag string `json:"tag"`
	} `json:"attrs"`
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
	default:
		return fmt.Sprint(x)
	}
}

// parseLog identifica o formato e devolve os campos.
func parseLog(log string) (format string, f fields) {
	m := strings.TrimSpace(log)
	switch {
	case strings.HasPrefix(m, "{"):
		var j fields
		if err := json.Unmarshal([]byte(m), &j); err != nil {
			return "json", fields{"msg": m}
		}
		// msg com pares embutidos: "create_start transaction_id=... latency_ms=20"
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
		for k, v := range parseCommaKV(outer["msg"]) {
			f[k] = v
		}
		return "logrus", f
	default:
		f := fields{}
		for k, v := range parseLogfmt(m) {
			f[k] = v
		}
		return "slog", f
	}
}

var (
	wordRe   = regexp.MustCompile(`^\w+$`)
	kvRe     = regexp.MustCompile(`\w=`)
	logfmtRe = regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|\S+)`)
	keyRe    = regexp.MustCompile(`^[\w.]+`)
)

// parseLogfmt: key=value | key="quoted \" value". Primeira ocorrência vence.
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

// tryJSON decodifica strings que parecem JSON; o resto volta como veio.
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

// dockerTS converte "2026-11-07T16:48:43.348796243Z" em ms com fração.
func dockerTS(s string) (float64, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, false
	}
	return float64(t.UnixNano()) / 1e6, true
}

var txSuffixRe = regexp.MustCompile(`-(\d{14})$`)

// CreatedAt extrai a hora de criação do sufixo YYYYMMDDhhmmss do transaction_id (horário de Brasília).
func CreatedAt(tx string) (time.Time, bool) {
	m := txSuffixRe.FindStringSubmatch(tx)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("20060102150405", m[1], brt)
	return t, err == nil
}

var brt = time.FixedZone("BRT", -3*3600)
