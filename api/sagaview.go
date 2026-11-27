package main

import (
	"encoding/json"
	"strings"
	"time"

	"sagalogs/internal/saga"
)

// buildSaga entrega as linhas da transação ao montador de sagas, no mesmo
// formato que ele lê do Loki (JSON do driver json-file com attrs.tag "imagem|app|sha|container").
func buildSaga(events []*Event) (*saga.Saga, error) {
	lines := make([]saga.RawLine, 0, len(events))
	for _, e := range events {
		b, _ := json.Marshal(map[string]any{
			"log":    e.Raw,
			"stream": e.Stream,
			"time":   e.TS.Format(time.RFC3339Nano),
			"attrs":  map[string]string{"tag": e.Image + "|" + e.Service + "||" + e.Container},
		})
		lines = append(lines, saga.RawLine{Line: string(b), TS: e.TS})
	}
	sg, err := saga.Build(lines)
	if err != nil {
		return nil, err
	}
	// sem gateway (mensagem publicada direto na fila): nomeia pela primeira etapa
	if sg.Operation == "" && len(sg.Steps) > 0 {
		st := sg.Steps[0]
		sg.Operation = strings.TrimSpace(st.Label + " · " + strings.TrimSpace(st.Action+" "+st.Resource))
		sg.Operation = strings.TrimSuffix(sg.Operation, " ·")
	}
	return sg, nil
}
