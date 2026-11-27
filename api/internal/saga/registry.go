package saga

import (
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Registro dinâmico das apps: nada fixo por nome.
//
// Precedência: label no compose (sagalogs.role/vendor/label) > papel aprendido pelos
// logs (assinaturas da lib core/v2, do gateway e do orquestrador) > padrão (worker).
// Vendor e rótulo saem do nome do serviço quando não vêm por label.

type override struct {
	role, vendor, label string
	final               bool
}

var reg = struct {
	sync.RWMutex
	over  map[string]override
	votes map[string]*[3]int // gateway, orchestrator, worker
}{over: map[string]override{}, votes: map[string]*[3]int{}}

var roleIdx = map[Role]int{RoleGateway: 0, RoleOrchestrator: 1, RoleWorker: 2}
var roleOf = [3]Role{RoleGateway, RoleOrchestrator, RoleWorker}

// SetOverride registra o que veio dos labels do container (vazio = sem override).
func SetOverride(app, role, vendor, label string, final bool) {
	reg.Lock()
	defer reg.Unlock()
	if role == "" && vendor == "" && label == "" && !final {
		delete(reg.over, app)
		return
	}
	reg.over[app] = override{strings.ToLower(role), strings.ToLower(vendor), label, final}
}

// Observe vota no papel da app a partir dos campos de uma linha de log já parseada.
func Observe(app string, f map[string]any) {
	r, ok := roleHint(fields(f))
	if !ok {
		return
	}
	reg.Lock()
	v := reg.votes[app]
	if v == nil {
		v = &[3]int{}
		reg.votes[app] = v
	}
	v[roleIdx[r]]++
	reg.Unlock()
}

// roleHint: linhas que só um papel escreve.
func roleHint(f fields) (Role, bool) {
	msg := f.str("msg")
	integ := f.str("integration")
	switch {
	case reStateIn.MatchString(msg), reStepEnd.MatchString(msg), reStart.MatchString(msg),
		msg == "Action execution failed", integ == "Log" && f.str("type") != "":
		return RoleWorker, true
	case msg == "request received" && f.str("path") != "",
		msg == "Relay message added", reOutboxRelay.MatchString(msg), reRelayFail.MatchString(msg):
		return RoleGateway, true
	case f.str("package") == "middlewares", f.str("package") == "handlers.callbacks",
		integ == "amqp" && strings.Contains(f.str("integration_args"), "RoutingKey"):
		return RoleOrchestrator, true
	}
	return "", false
}

func learnedRole(app string) (Role, string) {
	v := reg.votes[app]
	if v == nil {
		return RoleWorker, "padrão"
	}
	best := 2
	for i := range v {
		if v[i] > v[best] {
			best = i
		}
	}
	if v[best] == 0 {
		return RoleWorker, "padrão"
	}
	return roleOf[best], "logs"
}

func appInfo(name, image, version string) AppInfo {
	a, _ := Describe(name)
	a.Image, a.Version = image, version
	return a
}

// Describe devolve o que se sabe da app e de onde veio o papel (label, logs, padrão).
func Describe(name string) (AppInfo, string) {
	reg.RLock()
	defer reg.RUnlock()
	o := reg.over[name]
	role, src := learnedRole(name)
	if o.role != "" {
		role, src = Role(o.role), "label"
	}
	a := AppInfo{Name: name, Role: role, Vendor: o.vendor, Label: o.label, Final: o.final || (finalAppsRe != nil && finalAppsRe.MatchString(name))}
	if a.Vendor == "" {
		a.Vendor = guessVendor(name)
	}
	if a.Label == "" {
		a.Label = humanize(name)
	}
	return a, src
}

// KnownApps lista as apps com override ou votos.
func KnownApps() []string {
	reg.RLock()
	defer reg.RUnlock()
	seen := map[string]bool{}
	for k := range reg.over {
		seen[k] = true
	}
	for k := range reg.votes {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// vendors reconhecidos por palavra no nome (o ícone na UI depende disso).
var vendorWords = []struct{ word, vendor string }{
	{"fortinet", "fortinet"}, {"fortigate", "fortinet"},
	{"paloalto", "paloalto"}, {"panos", "paloalto"},
	{"nsxt", "nsxt"}, {"nsx", "nsxt"},
	{"ontap", "ontap"}, {"netapp", "ontap"},
	{"rubrik", "rubrik"},
	{"citrix", "citrix"},
	{"entraid", "entraid"}, {"entra", "entraid"}, {"azuread", "entraid"},
	{"vmware", "vmware"}, {"vsphere", "vmware"}, {"vcenter", "vmware"},
}

func guessVendor(app string) string {
	for _, w := range words(app) {
		for _, v := range vendorWords {
			if w == v.word {
				return v.vendor
			}
		}
	}
	return "interno"
}

var special = map[string]string{
	"api": "API", "nat": "NAT", "snat": "SNAT", "ip": "IP", "ippool": "IP Pool", "vm": "VM",
	"nsxt": "NSX-T", "nsx": "NSX", "daas": "DaaS", "entraid": "Entra ID", "vmware": "VMware",
	"vsphere": "vSphere", "ontap": "ONTAP", "paloalto": "Palo Alto", "dns": "DNS", "db": "DB",
	"http": "HTTP", "grpc": "gRPC", "aws": "AWS", "gcp": "GCP", "id": "ID", "ad": "AD", "ldap": "LDAP",
}

// humanize: "nsxt-nat" → "NSX-T NAT", "billing-invoice-api" → "Billing Invoice API".
func humanize(app string) string {
	ws := words(app)
	for i, w := range ws {
		if s, ok := special[w]; ok {
			ws[i] = s
		} else if w != "" {
			r := []rune(w)
			r[0] = unicode.ToUpper(r[0])
			ws[i] = string(r)
		}
	}
	if len(ws) == 0 {
		return app
	}
	return strings.Join(ws, " ")
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == ' ' })
}
