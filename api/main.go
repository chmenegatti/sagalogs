// sagalogs: visualizador local dos logs dos containers do docker compose.
// Lê direto do Docker (unix socket), agrupa por transaction_id e destaca o
// payload que chega em cada worker. Sem Loki, sem banco: buffer em memória.
package main

import (
	"sagalogs/internal/saga"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	addr := env("SAGALOGS_ADDR", ":15001")
	projects := splitList(env("SAGALOGS_PROJECTS", "")) // vazio = todos os projetos compose
	bufSize, _ := strconv.Atoi(env("SAGALOGS_BUFFER", "100000"))
	tail, _ := strconv.Atoi(env("SAGALOGS_TAIL", "2000"))
	uiDir := env("SAGALOGS_UI_DIR", filepath.Join(exeDir(), "..", "..", "ui", "dist"))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := saga.SetFinalApps(os.Getenv("SAGALOGS_FINAL_APPS")); err != nil {
		log.Error("SAGALOGS_FINAL_APPS inválida", "err", err)
		os.Exit(1)
	}
	st := newStore(bufSize)
	col := newCollector(newDocker(env("SAGALOGS_DOCKER_SOCK", "/var/run/docker.sock")), st, projects, tail, log)
	if f := os.Getenv("SAGALOGS_REPLAY"); f != "" {
		n, err := replay(st, f)
		log.Info("replay carregado", "arquivo", f, "linhas", n, "err", err)
	}
	if os.Getenv("SAGALOGS_DOCKER") != "0" {
		go col.run(ctx)
	}

	srv := &http.Server{Addr: addr, Handler: routes(st, col, uiDir)}
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	log.Info("sagalogs no ar", "addr", "http://localhost"+addr, "projetos", projects, "ui", uiDir)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http", "err", err)
		os.Exit(1)
	}
}

func routes(st *store, col *collector, uiDir string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, r *http.Request) {
		n, seq, txs := st.stats()
		writeJSON(w, map[string]any{"projects": col.getProjects(), "events": n, "seq": seq, "transactions": txs, "capacity": st.cap})
	})
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, col.projectList())
	})
	// PUT {"projects": ["a","b"]}; lista vazia = todos
	mux.HandleFunc("PUT /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Projects []string `json:"projects"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		col.setProjects(body.Projects)
		writeJSON(w, col.getProjects())
	})
	mux.HandleFunc("GET /api/apps", func(w http.ResponseWriter, r *http.Request) {
		type appView struct {
			saga.AppInfo
			Source string `json:"source"` // de onde veio o papel: label, logs ou padrão
		}
		out := []appView{}
		for _, a := range saga.KnownApps() {
			info, src := saga.Describe(a)
			out = append(out, appView{info, src})
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /api/containers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, col.containers())
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, st.query(parseFilter(r), intParam(r, "limit", 2000, 20000)))
	})
	mux.HandleFunc("GET /api/tx", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, st.transactions(strings.ToLower(r.URL.Query().Get("q")), intParam(r, "limit", 500, 5000)))
	})
	mux.HandleFunc("GET /api/tx/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, st.query(filter{tx: r.PathValue("id"), noise: true}, 20000))
	})
	mux.HandleFunc("GET /api/saga/{id}", func(w http.ResponseWriter, r *http.Request) {
		s, err := buildSaga(st.query(filter{tx: r.PathValue("id"), noise: true}, 20000))
		if err != nil {
			http.Error(w, "not-found", http.StatusNotFound)
			return
		}
		writeJSON(w, s)
	})
	mux.HandleFunc("POST /api/clear", func(w http.ResponseWriter, r *http.Request) {
		st.clear()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/stream", func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "sem streaming", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		c := st.subscribe()
		defer st.unsubscribe(c)
		fmt.Fprint(w, ": ok\n\n")
		fl.Flush()

		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()
		// agrupa rajadas: um flush a cada 100ms no máximo
		flush := time.NewTicker(100 * time.Millisecond)
		defer flush.Stop()
		dirty := false
		for {
			select {
			case <-r.Context().Done():
				return
			case e := <-c:
				b, _ := json.Marshal(e)
				fmt.Fprintf(w, "data: %s\n\n", b)
				dirty = true
			case <-flush.C:
				if dirty {
					fl.Flush()
					dirty = false
				}
			case <-ping.C:
				fmt.Fprint(w, ": ping\n\n")
				fl.Flush()
			}
		}
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.Handle("/", spa(uiDir))
	return mux
}

// spa serve ui/dist; rota desconhecida cai no index.html (router do React).
func spa(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
				http.Error(w, "UI não compilada: rode `bun run build` em ui/", http.StatusNotFound)
				return
			}
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fs.ServeHTTP(w, r)
	})
}

func parseFilter(r *http.Request) filter {
	q := r.URL.Query()
	set := func(k string) map[string]bool {
		v := splitList(q.Get(k))
		if len(v) == 0 {
			return nil
		}
		m := map[string]bool{}
		for _, x := range v {
			m[x] = true
		}
		return m
	}
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	return filter{
		containers: set("container"), levels: set("level"), kinds: set("kind"),
		tx: q.Get("tx"), q: strings.ToLower(q.Get("q")), noise: q.Get("noise") == "1", before: before,
	}
}

func intParam(r *http.Request, k string, def, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(k))
	if err != nil || v <= 0 {
		return def
	}
	return min(v, max)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}
