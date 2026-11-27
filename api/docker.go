package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"sagalogs/internal/saga"
)

// Cliente mínimo da Docker Engine API via unix socket (sem SDK).
type docker struct {
	hc *http.Client
}

func newDocker(sock string) *docker {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}
	return &docker{hc: &http.Client{Transport: tr}}
}

func (d *docker) get(ctx context.Context, path string, q url.Values) (*http.Response, error) {
	u := "http://docker" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := d.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		res.Body.Close()
		return nil, fmt.Errorf("docker %s: %d %s", path, res.StatusCode, strings.TrimSpace(string(b)))
	}
	return res, nil
}

type apiContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Image  string            `json:"Image"`
	Labels map[string]string `json:"Labels"`
}

func (c apiContainer) name() string {
	if len(c.Names) == 0 {
		return c.ID[:12]
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

// list devolve os containers (rodando ou não) dos projetos compose informados.
// Lista vazia = todos os projetos compose.
func (d *docker) list(ctx context.Context, projects []string) ([]apiContainer, error) {
	labels := []string{"com.docker.compose.project"}
	if len(projects) > 0 {
		labels = labels[:0]
		for _, p := range projects {
			labels = append(labels, "com.docker.compose.project="+p)
		}
	}
	var all []apiContainer
	// filtro de label com vários valores é AND no Docker: uma chamada por projeto
	for _, l := range labels {
		f, _ := json.Marshal(map[string][]string{"label": {l}})
		res, err := d.get(ctx, "/containers/json", url.Values{"all": {"1"}, "filters": {string(f)}})
		if err != nil {
			return nil, err
		}
		var cs []apiContainer
		err = json.NewDecoder(res.Body).Decode(&cs)
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		all = append(all, cs...)
	}
	return all, nil
}

func (d *docker) isTTY(ctx context.Context, id string) bool {
	res, err := d.get(ctx, "/containers/"+id+"/json", nil)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var v struct {
		Config struct{ Tty bool } `json:"Config"`
	}
	_ = json.NewDecoder(res.Body).Decode(&v)
	return v.Config.Tty
}

// follow lê os logs do container (com timestamps) até o stream fechar.
func (d *docker) follow(ctx context.Context, id string, since time.Time, tail int, emit func(stream, line string)) error {
	q := url.Values{"follow": {"1"}, "stdout": {"1"}, "stderr": {"1"}, "timestamps": {"1"}}
	if since.IsZero() {
		q.Set("tail", fmt.Sprint(tail))
	} else {
		// +1ns: não repetir a última linha já lida
		q.Set("since", fmt.Sprintf("%d.%09d", since.Unix(), since.Nanosecond()+1))
	}
	tty := d.isTTY(ctx, id)
	res, err := d.get(ctx, "/containers/"+id+"/logs", q)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if tty {
		return scanLines(res.Body, func(l string) { emit("stdout", l) })
	}
	// stream multiplexado: [stream(1) 0 0 0 size(4 BE)] payload
	pw := map[byte]*io.PipeWriter{}
	var wg sync.WaitGroup
	for _, s := range []byte{1, 2} {
		pr, w := io.Pipe()
		pw[s] = w
		name := map[byte]string{1: "stdout", 2: "stderr"}[s]
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = scanLines(pr, func(l string) { emit(name, l) })
		}()
	}
	defer func() {
		for _, w := range pw {
			w.Close()
		}
		wg.Wait()
	}()
	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(res.Body, hdr); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}
		n := int64(binary.BigEndian.Uint32(hdr[4:]))
		w := pw[hdr[0]]
		if w == nil {
			w = pw[1]
		}
		if _, err := io.CopyN(w, res.Body, n); err != nil {
			return err
		}
	}
}

func scanLines(r io.Reader, fn func(string)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024) // payloads grandes numa linha só
	for sc.Scan() {
		fn(sc.Text())
	}
	return sc.Err()
}

// collector descobre containers e mantém um follower por container rodando.
// Os projetos seguidos mudam em tempo de execução (seletor na UI).
type collector struct {
	d     *docker
	store *store
	tail  int
	log   *slog.Logger

	mu       sync.Mutex
	projects []string                      // vazio = todos os projetos compose
	attached map[string]context.CancelFunc // id → follower ativo
	lastTS   map[string]time.Time          // id → última linha lida (retomada após restart)
	info     map[string]apiContainer       // containers dos projetos seguidos
	all      map[string]apiContainer       // todos os containers compose (para o seletor)
	kick     chan struct{}
}

func newCollector(d *docker, st *store, projects []string, tail int, log *slog.Logger) *collector {
	return &collector{
		d: d, store: st, projects: projects, tail: tail, log: log,
		attached: map[string]context.CancelFunc{}, lastTS: map[string]time.Time{},
		info: map[string]apiContainer{}, all: map[string]apiContainer{}, kick: make(chan struct{}, 1),
	}
}

func (c *collector) run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		c.sync(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-c.kick:
		}
	}
}

func (c *collector) setProjects(p []string) {
	c.mu.Lock()
	c.projects = p
	c.mu.Unlock()
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

func (c *collector) getProjects() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.projects...)
}

func project(ct apiContainer) string { return ct.Labels["com.docker.compose.project"] }
func service(ct apiContainer) string {
	if s := ct.Labels["com.docker.compose.service"]; s != "" {
		return s
	}
	return ct.name()
}
func ignored(ct apiContainer) bool { return truthy(ct.Labels["sagalogs.ignore"]) }
func truthy(v string) bool {
	v = strings.ToLower(v)
	return v == "true" || v == "1" || v == "yes"
}

func (c *collector) sync(ctx context.Context) {
	cs, err := c.d.list(ctx, nil)
	if err != nil {
		c.log.Warn("docker list", "err", err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	want := map[string]bool{}
	for _, p := range c.projects {
		want[p] = true
	}
	c.all = map[string]apiContainer{}
	seen := map[string]bool{}
	for _, ct := range cs {
		c.all[ct.ID] = ct
		if len(want) > 0 && !want[project(ct)] {
			continue
		}
		seen[ct.ID] = true
		c.info[ct.ID] = ct
		// labels sagalogs.* do compose sobrepõem o que foi aprendido pelos logs
		saga.SetOverride(service(ct), ct.Labels["sagalogs.role"], ct.Labels["sagalogs.vendor"], ct.Labels["sagalogs.label"], truthy(ct.Labels["sagalogs.final"]))
		if ct.State != "running" || c.attached[ct.ID] != nil || ignored(ct) {
			continue
		}
		fctx, cancel := context.WithCancel(ctx)
		c.attached[ct.ID] = cancel
		go c.followOne(fctx, ct, c.lastTS[ct.ID])
	}
	for id := range c.info {
		if !seen[id] {
			delete(c.info, id)
			if cancel := c.attached[id]; cancel != nil {
				cancel() // projeto saiu da seleção ou container removido
				delete(c.attached, id)
			}
		}
	}
}

func (c *collector) followOne(ctx context.Context, ct apiContainer, since time.Time) {
	name, svc := ct.name(), service(ct)
	c.log.Info("seguindo logs", "container", name)
	err := c.d.follow(ctx, ct.ID, since, c.tail, func(stream, line string) {
		ts, rest := splitTS(line)
		e := parseLine(name, svc, stream, ts, rest)
		e.Image = ct.Image
		c.store.add(e)
		c.mu.Lock()
		c.lastTS[ct.ID] = ts
		c.mu.Unlock()
	})
	if err != nil && ctx.Err() == nil {
		c.log.Warn("logs encerrados", "container", name, "err", err)
	}
	c.mu.Lock()
	if ctx.Err() == nil { // terminou sozinho (container parou): libera para reanexar
		delete(c.attached, ct.ID)
	}
	c.mu.Unlock()
}

type projectView struct {
	Name       string `json:"name"`
	Containers int    `json:"containers"`
	Running    int    `json:"running"`
	Selected   bool   `json:"selected"`
	WorkingDir string `json:"workingDir,omitempty"`
}

func (c *collector) projectList() []projectView {
	c.mu.Lock()
	defer c.mu.Unlock()
	want := map[string]bool{}
	for _, p := range c.projects {
		want[p] = true
	}
	m := map[string]*projectView{}
	for _, ct := range c.all {
		p := project(ct)
		v := m[p]
		if v == nil {
			v = &projectView{Name: p, Selected: len(want) == 0 || want[p], WorkingDir: ct.Labels["com.docker.compose.project.working_dir"]}
			m[p] = v
		}
		v.Containers++
		if ct.State == "running" {
			v.Running++
		}
	}
	// projeto escolhido que não está de pé continua aparecendo
	for p := range want {
		if m[p] == nil {
			m[p] = &projectView{Name: p, Selected: true}
		}
	}
	out := make([]projectView, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func splitTS(line string) (time.Time, string) {
	if sp := strings.IndexByte(line, ' '); sp > 0 {
		if t, err := time.Parse(time.RFC3339Nano, line[:sp]); err == nil {
			return t, line[sp+1:]
		}
	}
	return time.Now(), line
}

type containerView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Service  string `json:"service"`
	State    string `json:"state"`
	Status   string `json:"status"`
	Image    string `json:"image"`
	Project  string `json:"project"`
	Attached bool   `json:"attached"`
	Count    int    `json:"count"`
}

func (c *collector) containers() []containerView {
	c.mu.Lock()
	defer c.mu.Unlock()
	counts := c.store.countsByContainer()
	out := make([]containerView, 0, len(c.info))
	for id, ct := range c.info {
		out = append(out, containerView{
			ID: id[:12], Name: ct.name(), Service: ct.Labels["com.docker.compose.service"],
			State: ct.State, Status: ct.Status, Image: ct.Image,
			Project: project(ct), Attached: c.attached[id] != nil, Count: counts[ct.name()],
		})
	}
	return out
}
