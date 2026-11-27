package main

import (
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// store: ring buffer em memória + índice por transaction_id + assinantes do SSE.
type store struct {
	mu   sync.RWMutex
	buf  []*Event
	cap  int
	seq  int64
	subs map[chan *Event]struct{}
	txs  map[string]*TxSummary
}

type TxSummary struct {
	Tx       string    `json:"tx"`
	First    time.Time `json:"first"`
	Last     time.Time `json:"last"`
	Events   int       `json:"events"`
	Received int       `json:"received"` // payloads recebidos por workers
	Failed   int       `json:"failed"`
	Errors   int       `json:"errors"`
	Services []string  `json:"services"`
	Actions  []string  `json:"actions"`
	Entry    string    `json:"entry,omitempty"` // primeira requisição HTTP (ex.: POST /v1/identities)
	LastStep string    `json:"lastStep,omitempty"`
	LastKind Kind      `json:"lastKind"`
}

func newStore(capacity int) *store {
	return &store{cap: capacity, subs: map[chan *Event]struct{}{}, txs: map[string]*TxSummary{}}
}

func (s *store) add(e *Event) {
	s.mu.Lock()
	s.seq++
	e.Seq = s.seq
	if len(s.buf) >= s.cap {
		old := s.buf[0]
		s.buf = s.buf[1:]
		if old.Tx != "" {
			if t := s.txs[old.Tx]; t != nil {
				t.Events--
				if t.Events <= 0 {
					delete(s.txs, old.Tx)
				}
			}
		}
	}
	s.buf = append(s.buf, e)
	if e.Tx != "" {
		s.index(e)
	}
	subs := make([]chan *Event, 0, len(s.subs))
	for c := range s.subs {
		subs = append(subs, c)
	}
	s.mu.Unlock()

	for _, c := range subs {
		select {
		case c <- e:
		default: // cliente lento: perde o evento, não trava a coleta
		}
	}
}

func (s *store) index(e *Event) {
	t := s.txs[e.Tx]
	if t == nil {
		t = &TxSummary{Tx: e.Tx, First: e.TS}
		s.txs[e.Tx] = t
	}
	t.Events++
	if e.TS.Before(t.First) {
		t.First = e.TS
	}
	if e.TS.After(t.Last) {
		t.Last = e.TS
	}
	if !slices.Contains(t.Services, e.Service) {
		t.Services = append(t.Services, e.Service)
	}
	switch e.Kind {
	case KindReceived:
		t.Received++
		if e.Action != "" && !slices.Contains(t.Actions, e.Action) {
			t.Actions = append(t.Actions, e.Action)
		}
	case KindFailed:
		t.Failed++
	case KindError:
		t.Errors++
	default:
		if e.Error != "" { // WARN com erro (ex.: relay do outbox tentando de novo)
			t.Errors++
		}
	case KindHTTPIn:
		if t.Entry == "" {
			t.Entry = e.Target
		}
	}
	if e.Step != "" {
		t.LastStep = e.Step
	}
	t.LastKind = e.Kind
}

func (s *store) subscribe() chan *Event {
	c := make(chan *Event, 4096)
	s.mu.Lock()
	s.subs[c] = struct{}{}
	s.mu.Unlock()
	return c
}

func (s *store) unsubscribe(c chan *Event) {
	s.mu.Lock()
	delete(s.subs, c)
	s.mu.Unlock()
}

func (s *store) clear() {
	s.mu.Lock()
	s.buf = nil
	s.txs = map[string]*TxSummary{}
	s.mu.Unlock()
}

type filter struct {
	containers map[string]bool
	tx         string
	q          string
	levels     map[string]bool
	kinds      map[string]bool
	noise      bool
	before     int64
}

func (f filter) match(e *Event) bool {
	if f.before > 0 && e.Seq >= f.before {
		return false
	}
	if len(f.containers) > 0 && !f.containers[e.Container] {
		return false
	}
	if f.tx != "" && e.Tx != f.tx {
		return false
	}
	if !f.noise && e.Noise {
		return false
	}
	if len(f.levels) > 0 && !f.levels[e.Level] {
		return false
	}
	if len(f.kinds) > 0 && !f.kinds[string(e.Kind)] {
		return false
	}
	if f.q != "" && !strings.Contains(strings.ToLower(e.Raw), f.q) {
		return false
	}
	return true
}

// query devolve os últimos `limit` eventos que casam, em ordem cronológica.
func (s *store) query(f filter, limit int) []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Event, 0, min(limit, 512))
	for i := len(s.buf) - 1; i >= 0 && len(out) < limit; i-- {
		if f.match(s.buf[i]) {
			out = append(out, s.buf[i])
		}
	}
	slices.Reverse(out)
	return out
}

func (s *store) transactions(q string, limit int) []TxSummary {
	s.mu.RLock()
	out := make([]TxSummary, 0, len(s.txs))
	for _, t := range s.txs {
		if q != "" && !strings.Contains(strings.ToLower(t.Tx+" "+t.Entry+" "+strings.Join(t.Services, " ")), q) {
			continue
		}
		c := *t
		c.Services = slices.Clone(t.Services)
		c.Actions = slices.Clone(t.Actions)
		out = append(out, c)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Last.After(out[j].Last) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *store) countsByContainer() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := map[string]int{}
	for _, e := range s.buf {
		m[e.Container]++
	}
	return m
}

func (s *store) stats() (events int, seq int64, txs int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.buf), s.seq, len(s.txs)
}
