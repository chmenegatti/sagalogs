package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"
)

// replay carrega um export do Grafana/Loki (linhas "ts<TAB>{docker json}") no buffer.
// Serve para ver a UI com uma saga real sem disparar nada nos containers.
func replay(st *store, path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	type dl struct {
		Log    string `json:"log"`
		Stream string `json:"stream"`
		Time   string `json:"time"`
		Attrs  struct {
			Tag string `json:"tag"`
		} `json:"attrs"`
	}
	var evs []*Event
	for _, line := range strings.Split(string(b), "\n") {
		_, js, ok := strings.Cut(line, "\t")
		if !ok || !strings.HasPrefix(js, "{") {
			continue
		}
		var d dl
		if json.Unmarshal([]byte(js), &d) != nil {
			continue
		}
		app := d.Attrs.Tag
		if parts := strings.Split(d.Attrs.Tag, "|"); len(parts) > 1 {
			app = parts[1]
		}
		ts, err := time.Parse(time.RFC3339Nano, d.Time)
		if err != nil {
			ts = time.Now()
		}
		evs = append(evs, parseLine(app, app, d.Stream, ts, strings.TrimRight(d.Log, "\n")))
	}
	// o export vem do mais novo para o mais antigo
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].TS.Before(evs[j].TS) })
	for _, e := range evs {
		st.add(e)
	}
	return len(evs), nil
}
