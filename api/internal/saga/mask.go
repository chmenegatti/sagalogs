package saga

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// Mascaramento de segredos (senha, PSK, token…) antes de qualquer saída da API.
//
// Valor mascarado = Masked + "#" + 6 hex de HMAC-SHA256 com chave aleatória do processo:
// o mesmo valor dá a mesma marca (o diff "O que mudou" ainda acusa troca), mas a marca
// não permite recuperar o valor nem comparar entre instâncias/reinícios.
const Masked = "••••••"

// DefaultMaskKeys casa com o NOME da chave (case-insensitive, em qualquer parte do nome).
const DefaultMaskKeys = `(?i)(password|passwd|passphrase|senha|secret|token|psk|pre_?shared|api_?key|private_?key|authorization|credential)`

var (
	maskKey   = func() []byte { b := make([]byte, 32); _, _ = rand.Read(b); return b }()
	maskKeyRe = regexp.MustCompile(DefaultMaskKeys)
	// key=valor, "key":"valor" e \"key\":\"valor\" em texto livre (linha crua, mensagem, erro)
	maskTextRe = buildTextRe(DefaultMaskKeys)
	// "Bearer xyz" / "Basic xyz" em qualquer lugar do texto
	authSchemeRe = regexp.MustCompile(`(?i)\b(bearer|basic)\s+([A-Za-z0-9._~+/=-]{8,})`)
)

// SetMaskKeys troca o padrão de chaves sensíveis . Chamar na partida.
func SetMaskKeys(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	maskKeyRe, maskTextRe = re, buildTextRe(pattern)
	return nil
}

func buildTextRe(keys string) *regexp.Regexp {
	inner := strings.TrimPrefix(keys, "(?i)")
	// grupo 1: nome da chave + separador (com aspas escapadas ou não); grupo 2: o valor
	return regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*` + inner + `[A-Za-z0-9_.-]*(?:\\*")?\s*[:=]\s*(?:\\*")?)([^"\\\s,;&}{\[][^"\\\s,;&}]*)`)
}

func tag(v string) string {
	m := hmac.New(sha256.New, maskKey)
	m.Write([]byte(v))
	return Masked + "#" + hex.EncodeToString(m.Sum(nil))[:6]
}

func isMasked(v string) bool { return strings.HasPrefix(v, Masked) }

// masker guarda os valores achados nos campos estruturados para apagá-los também
// do texto livre, onde aparecem com escapes que a regex não pega.
type masker struct{ found map[string]string }

func (m *masker) value(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if maskKeyRe.MatchString(k) {
				x[k] = m.secret(val)
			} else {
				x[k] = m.value(val)
			}
		}
		return x
	case []any:
		for i := range x {
			x[i] = m.value(x[i])
		}
		return x
	case string:
		// JSON dentro de string (payload que veio escapado)
		if t := strings.TrimSpace(x); len(t) > 1 && (t[0] == '{' || t[0] == '[') {
			var inner any
			if json.Unmarshal([]byte(t), &inner) == nil {
				if b, err := json.Marshal(m.value(inner)); err == nil {
					return string(b)
				}
			}
		}
		return m.text(x)
	}
	return v
}

// secret mascara o valor de uma chave sensível. Estruturas (ex.: sql.NullString
// {"String":"…","Valid":true}) são percorridas: escalares viram marca, booleanos ficam.
func (m *masker) secret(v any) any {
	switch x := v.(type) {
	case nil, bool: // flag (ex.: IsFirstChangeUserPassword) não é segredo
		return v
	case string:
		if x == "" || isMasked(x) {
			return x
		}
		t := tag(x)
		m.found[x] = t
		return t
	case map[string]any:
		for k, val := range x {
			x[k] = m.secret(val)
		}
		return x
	case []any:
		for i := range x {
			x[i] = m.secret(x[i])
		}
		return x
	default:
		b, _ := json.Marshal(x)
		return tag(string(b))
	}
}

func (m *masker) text(s string) string {
	if s == "" {
		return s
	}
	s = maskTextRe.ReplaceAllStringFunc(s, func(match string) string {
		// o padrão de chaves pode ter grupos próprios: o valor é sempre o último grupo
		sub := maskTextRe.FindStringSubmatch(match)
		val := sub[len(sub)-1]
		switch v := strings.ToLower(val); {
		case isMasked(val), v == "true", v == "false", v == "null":
			return match
		}
		return match[:len(match)-len(val)] + tag(val)
	})
	return authSchemeRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := authSchemeRe.FindStringSubmatch(match)
		return sub[1] + " " + tag(sub[2])
	})
}

// literals apaga do texto livre os segredos achados nos campos estruturados,
// nas formas crua e com escape JSON (1 e 2 níveis). Mais longos primeiro.
func (m *masker) literals(s string) string {
	if len(m.found) == 0 || s == "" {
		return s
	}
	keys := make([]string, 0, len(m.found))
	for k := range m.found {
		if len(k) >= 4 { // valores curtos demais casariam por acaso
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		t := m.found[k]
		e1, _ := json.Marshal(k)
		esc1 := string(e1[1 : len(e1)-1])
		e2, _ := json.Marshal(esc1)
		esc2 := string(e2[1 : len(e2)-1])
		for _, form := range []string{esc2, esc1, k} {
			s = strings.ReplaceAll(s, form, t)
		}
	}
	return s
}

// MaskSaga mascara a saga no lugar. Idempotente.
func MaskSaga(s *Saga) {
	m := &masker{found: map[string]string{}}
	for _, st := range s.Steps {
		st.PayloadIn = m.value(st.PayloadIn)
		st.PayloadOut = m.value(st.PayloadOut)
		if st.Broker != nil {
			m.value(st.Broker)
		}
		for i := range st.Calls {
			st.Calls[i].Payload = m.value(st.Calls[i].Payload)
		}
		for i := range st.JobCalls {
			st.JobCalls[i].Payload = m.value(st.JobCalls[i].Payload)
		}
	}
	for _, e := range s.Events {
		e.Payload = m.value(e.Payload)
		if e.Broker != nil {
			m.value(e.Broker)
		}
	}
	// texto livre por último, já conhecendo os valores dos campos estruturados
	for _, e := range s.Events {
		e.Raw = m.literals(m.text(e.Raw))
		e.Msg = m.literals(m.text(e.Msg))
		e.Error = m.literals(m.text(e.Error))
		e.Target = m.literals(m.text(e.Target))
	}
	for _, st := range s.Steps {
		st.Endpoint = m.literals(m.text(st.Endpoint))
	}
}
