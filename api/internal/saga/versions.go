package saga

import (
	"slices"
	"strings"
)

// VersionStat: uma versão (imagem) de uma aplicação no período. Contagens vêm de métricas do Loki.
type VersionStat struct {
	App   string `json:"app"`
	Image string `json:"image"` // registry/x:tag
	Tag   string `json:"tag"`   // tag sem o sufixo _<app> (o id do pipeline)
	// Execs: execuções do worker ("Setting up worker"); Failed: publicou ERROR; Errors: linhas de erro com transação
	Execs     int     `json:"execs"`
	Failed    int     `json:"failed"`
	Errors    int     `json:"errors"`
	FirstSeen float64 `json:"firstSeen"` // epoch ms da primeira linha no período
	LastSeen  float64 `json:"lastSeen"`
	// Since: a versão já rodava antes do começo do período (FirstSeen é só o começo da janela)
	Since bool `json:"since,omitempty"`
}

// AppVersions: as versões de uma aplicação, da mais antiga para a mais nova.
type AppVersions struct {
	App      string        `json:"app"`
	Versions []VersionStat `json:"versions"`
	// Worse: a versão mais nova falha bem mais que a anterior (ver Worse())
	Worse bool `json:"worse"`
}

// Regras de "piorou depois do deploy": precisa de volume para não alarmar com 1 falha em 3 execuções.
const (
	worseMinExecs   = 20
	worseMinFailed  = 3
	worseRatio      = 2.0
	worseMinRateGap = 0.02 // 2 pontos percentuais
)

// TagOf: "registry.example.com/acme/vmware-disk:2976428021_vmware-disk" → "2976428021".
func TagOf(image string) string {
	tag := image
	if i := strings.LastIndexByte(image, ':'); i >= 0 {
		tag = image[i+1:]
	}
	if i := strings.IndexByte(tag, '_'); i > 0 {
		tag = tag[:i]
	}
	return tag
}

func rate(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// worse: b (mais nova) falha bem mais que a (anterior), com volume suficiente nas duas.
func worse(a, b VersionStat) bool {
	if a.Execs < worseMinExecs || b.Execs < worseMinExecs || b.Failed < worseMinFailed {
		return false
	}
	ra, rb := rate(a.Failed, a.Execs), rate(b.Failed, b.Execs)
	return rb >= ra*worseRatio && rb-ra >= worseMinRateGap
}

// Versions agrupa por aplicação. Só aplicações com execuções de worker (o gateway não loga "Setting up worker").
// Primeiro as que pioraram, depois as que trocaram de versão no período, depois por taxa de falha.
func Versions(stats []VersionStat) []AppVersions {
	by := map[string][]VersionStat{}
	for _, v := range stats {
		v.Tag = TagOf(v.Image)
		by[v.App] = append(by[v.App], v)
	}
	var out []AppVersions
	for app, vs := range by {
		execs := 0
		for _, v := range vs {
			execs += v.Execs
		}
		if execs == 0 {
			continue
		}
		slices.SortFunc(vs, func(a, b VersionStat) int { return cmpF(a.FirstSeen, b.FirstSeen) })
		av := AppVersions{App: app, Versions: vs}
		if n := len(vs); n >= 2 {
			av.Worse = worse(vs[n-2], vs[n-1])
		}
		out = append(out, av)
	}
	latestRate := func(a AppVersions) float64 {
		v := a.Versions[len(a.Versions)-1]
		return rate(v.Failed, v.Execs)
	}
	slices.SortFunc(out, func(a, b AppVersions) int {
		if a.Worse != b.Worse {
			if a.Worse {
				return -1
			}
			return 1
		}
		if (len(a.Versions) > 1) != (len(b.Versions) > 1) {
			if len(a.Versions) > 1 {
				return -1
			}
			return 1
		}
		if c := cmpF(latestRate(b), latestRate(a)); c != 0 {
			return c
		}
		return strings.Compare(a.App, b.App)
	})
	return out
}
