package saga

import (
	"cmp"
	"slices"
)

// Regras de volume fora do normal: na janela recente, pelo menos volMin ações, volRatio vezes a média da base para
// janelas do mesmo tamanho e volPeak vezes a maior janela da base (organizações que rodam lotes grandes de vez em
// quando não disparam a cada lote). Sem nenhuma ação na base, é "novo". Calibrado no TESP7 (12-15/11/2026): com 20
// e 3× disparava em 19 de 72 horas, quase tudo lote normal de Terraform; assim, em 7, todas picos de verdade.
const (
	volMin   = 40
	volRatio = 5.0
	volPeak  = 1.5
	// volShare: um par (operação, organização) que responde por esta fração do pico já explica a operação e a
	// organização sozinhas (um cliente em loop vira um alarme só)
	volShare = 0.8
)

// VolumeSpike: uma operação, organização ou par das duas com volume fora do normal.
type VolumeSpike struct {
	Operation string  `json:"operation,omitempty"`
	Org       string  `json:"org,omitempty"`
	Count     int     `json:"count"`     // ações na janela recente
	PerWindow float64 `json:"perWindow"` // média da base por janela do mesmo tamanho
	Peak      int     `json:"peak"`      // maior janela da base
	Ratio     float64 `json:"ratio"`     // Count / PerWindow; 0 = novo
	New       bool    `json:"new,omitempty"`
	// usuário com mais ações na janela (cliente em loop) e quantas
	TopUser  string `json:"topUser,omitempty"`
	TopCount int    `json:"topCount,omitempty"`
}

type volKey struct{ op, org string }

// VolumeSpikes compara as ações da janela recente [from, to] com as da base [baseFrom, from). Ordem: novos e depois
// as maiores razões.
func VolumeSpikes(reqs []Request, baseFrom, from, to float64) []VolumeSpike {
	windows := (from - baseFrom) / max(to-from, 1)
	recent, base := map[volKey]int{}, map[volKey]int{}
	users := map[volKey]map[string]int{}
	slots := map[volKey]map[int]int{} // ações da base por janela (contada a partir de from, para trás)
	size := max(to-from, 1)
	for _, q := range reqs {
		if q.TS < baseFrom || q.TS > to || q.Operation == "" {
			continue
		}
		keys := []volKey{{op: q.Operation}}
		if q.Org != "" {
			keys = append(keys, volKey{org: q.Org}, volKey{q.Operation, q.Org})
		}
		for _, k := range keys {
			if q.TS < from {
				base[k]++
				if slots[k] == nil {
					slots[k] = map[int]int{}
				}
				slots[k][int((from-q.TS)/size)]++
				continue
			}
			recent[k]++
			if q.User != "" {
				if users[k] == nil {
					users[k] = map[string]int{}
				}
				users[k][q.User]++
			}
		}
	}
	flagged := map[volKey]VolumeSpike{}
	for k, n := range recent {
		if n < volMin {
			continue
		}
		per := float64(base[k]) / max(windows, 1)
		s := VolumeSpike{Operation: k.op, Org: k.org, Count: n, PerWindow: per}
		for _, c := range slots[k] {
			s.Peak = max(s.Peak, c)
		}
		switch {
		case base[k] == 0:
			s.New = true
		case float64(n) >= volRatio*per && float64(n) > volPeak*float64(s.Peak):
			s.Ratio = float64(n) / per
		default:
			continue
		}
		for u, c := range users[k] {
			if c > s.TopCount || c == s.TopCount && u < s.TopUser {
				s.TopUser, s.TopCount = u, c
			}
		}
		flagged[k] = s
	}
	out := []VolumeSpike{}
	for k, s := range flagged {
		if k.op == "" || k.org == "" {
			// sozinho, se um par marcado já explica o pico
			explained := false
			for p, ps := range flagged {
				if p.op != "" && p.org != "" && (p.op == k.op && k.org == "" || p.org == k.org && k.op == "") &&
					float64(ps.Count) >= volShare*float64(s.Count) {
					explained = true
					break
				}
			}
			if explained {
				continue
			}
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b VolumeSpike) int {
		if a.New != b.New {
			if a.New {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(b.Ratio, a.Ratio); c != 0 {
			return c
		}
		return cmp.Compare(b.Count, a.Count)
	})
	return out
}
