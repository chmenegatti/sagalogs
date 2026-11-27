package saga

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ErrorCause: de onde o erro veio, para decidir para quem abrir o chamado.
type ErrorCause struct {
	// Kind: credential | timeout | concurrency | infra | vendor | ours | retries | other
	Kind string `json:"kind"`
	// Vendor: fabricante que respondeu (pela assinatura do erro ou pelo nome do worker); vazio = nosso lado
	Vendor string `json:"vendor,omitempty"`
	// Reply: a mensagem que o fornecedor devolveu, quando vem um JSON com "message" dentro do erro
	Reply string `json:"reply,omitempty"`
}

var causeRules = []struct {
	kind string
	re   *regexp.Regexp
}{
	// a ordem importa: "timeout waiting for lock" é concorrência, não tempo esgotado
	{"concurrency", regexp.MustCompile(`(?i)deadlock|lock wait|timeout waiting for lock|TransactionRaceLost|race lost|try restarting transaction`)},
	// fila, banco, rede, configuração do próprio serviço: antes de "fornecedor" (um worker fortinet-* sem fila
	// não é culpa da Fortinet)
	{"infra", regexp.MustCompile(`(?i)\bAMQP\b|rabbitmq|broker|connection refused|connection reset|no such host|dial tcp|broken pipe|\betcd\b|too many connections|unexpected EOF|i/o timeout`)},
	{"credential", regexp.MustCompile(`(?i)password is (invalid|incorrect)|access token is (invalid|expired)|token (is )?expired|unauthori[sz]ed|forbidden|authentication failed|invalid credentials|\b(HTTP|status:?) ?40[13]\b`)},
	{"timeout", regexp.MustCompile(`(?i)time ?out|timed out|deadline exceeded|context canceled|not ready after|stuck in status`)},
	{"vendor", regexp.MustCompile(`(?i)fault=\w+|\bRBK\d+|Rubrik agent|RMAN|\bORA-\d+|graphql:|vCenter|\bStatus: [45]\d\d\b|Internal Server Error|Bad Gateway|Service Unavailable|status="error"|A specified parameter was not correct|Status: Failed, JobID`)},
	{"ours", regexp.MustCompile(`(?i)in database|bad state|resolves to multiple|sql:|\bError \d{4} \(|not identification|\bnot found\b|must be|não é permitid|is required|invalid configuration|already exists|nil pointer|index out of range`)},
	{"retries", regexp.MustCompile(`(?i)redelivery max attempts|very long redelivery loop|attempt threshold`)},
}

var vendorSignatures = []struct {
	vendor string
	re     *regexp.Regexp
}{
	{"VMware", regexp.MustCompile(`fault=\w+|vCenter|VMWare|VMware`)},
	{"Rubrik", regexp.MustCompile(`\bRBK\d+|Rubrik|\[rubrik-`)},
	{"Oracle", regexp.MustCompile(`\bORA-\d+|RMAN`)},
}

var vendorByPrefix = map[string]string{
	"vmware":   "VMware",
	"rubrik":   "Rubrik",
	"paloalto": "Palo Alto",
	"fortinet": "Fortinet",
	"nsxt":     "NSX-T",
	"ontap":    "NetApp ONTAP",
	"jcstack":  "JCStack",
}

var jsonStart = regexp.MustCompile(`\{"`)

// ClassifyError diz a causa provável de uma mensagem de erro do worker app.
func ClassifyError(msg, app string) ErrorCause {
	core := normWrapper.ReplaceAllString(normPublished.ReplaceAllString(msg, ""), "")
	c := ErrorCause{Kind: "other"}
	for _, r := range causeRules {
		if r.re.MatchString(core) {
			c.Kind = r.kind
			break
		}
	}
	if c.Kind != "ours" && c.Kind != "infra" {
		for _, v := range vendorSignatures {
			if v.re.MatchString(core) {
				c.Vendor = v.vendor
				break
			}
		}
		if c.Vendor == "" {
			p, _, _ := strings.Cut(app, "-")
			c.Vendor = vendorByPrefix[p]
		}
	}
	// erro "nosso" com cara de fornecedor (ex.: 404 do GraphQL da Rubrik) já caiu em vendor pela ordem
	if loc := jsonStart.FindStringIndex(core); loc != nil {
		var body struct {
			Message string `json:"message"`
		}
		dec := json.NewDecoder(strings.NewReader(core[loc[0]:]))
		if dec.Decode(&body) == nil && body.Message != "" {
			c.Reply = body.Message
		}
	}
	return c
}
