package saga

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var ErrNoLines = errors.New("nenhuma linha de log para esta transação")

var kindPrio = map[EventKind]int{KindStepStart: 0, KindStateIn: 1, KindIntegrationCall: 2, KindStepEnd: 3, KindPublish: 4}

func prio(k EventKind) int {
	if p, ok := kindPrio[k]; ok {
		return p
	}
	return 2
}

// Build monta a saga a partir das linhas cruas (qualquer ordem).
// Linhas de outras transações devem ser filtradas antes; linhas inválidas são ignoradas.
func Build(lines []RawLine) (*Saga, error) {
	var events []*Event
	apps := map[string]AppInfo{}
	var appOrder []string
	txID := ""

	for _, l := range lines {
		e, info, tx, ok := parseLine(l)
		if !ok {
			continue
		}
		if _, seen := apps[e.App]; !seen {
			apps[e.App] = info
			appOrder = append(appOrder, e.App)
		}
		if tx != "" && txID == "" {
			txID = tx
		}
		events = append(events, e)
	}
	if len(events) == 0 {
		return nil, ErrNoLines
	}

	slices.SortStableFunc(events, func(a, b *Event) int {
		if a.TS != b.TS {
			return cmpF(a.TS, b.TS)
		}
		return prio(a.Kind) - prio(b.Kind)
	})
	for i, e := range events {
		e.ID = fmt.Sprintf("e%d", i)
	}

	b := &builder{apps: apps, events: events}
	b.gateway()
	create := b.orchestrator()
	b.workers()
	b.jobCalls(create)
	b.finish()

	s := &Saga{
		TransactionID: txID,
		Steps:         b.steps,
		Edges:         b.edges,
		Events:        events,
		Operation:     b.operation,
		Org:           b.org,
		User:          b.user,
		Status:        "ok",
	}
	for _, name := range appOrder {
		s.Apps = append(s.Apps, apps[name])
	}
	// a UI faz .map nas listas: requisição que não virou saga não pode sair com null
	if s.Steps == nil {
		s.Steps = []*Step{}
	}
	if s.Edges == nil {
		s.Edges = []Edge{}
	}
	if s.Apps == nil {
		s.Apps = []AppInfo{}
	}
	if t, ok := CreatedAt(txID); ok {
		s.CreatedAt = t.Format("2006-01-02T15:04:05-07:00")
	}
	if len(s.Steps) > 0 {
		s.Start = s.Steps[0].Start
		s.End = s.Steps[len(s.Steps)-1].End
		for _, st := range s.Steps {
			s.End = math.Max(s.End, st.End)
		}
		s.DurationMs = math.Round(s.End - s.Start)
	}
	collectErrors(s)
	s.Status = sagaStatus(s.Steps)
	isSaga := false
	for _, st := range s.Steps {
		if st.Kind == StepWorker && IsFinalApp(st.App) {
			s.Finished = true
		}
		if st.Kind == StepOutbox {
			isSaga = true
		}
	}
	// sem worker final configurado (label/env): terminou se a última etapa é um worker que já publicou o resultado
	if !s.Finished && !finalConfigured() && len(s.Steps) > 0 {
		if last := s.Steps[len(s.Steps)-1]; last.Kind == StepWorker && last.PublishStatus != "" && last.PublishStatus != "IN_PROGRESS" {
			s.Finished = true
		}
	}
	// virou saga, nenhum worker final processou e nada falhou: em andamento (ou parada, pelo tempo sem atividade)
	if isSaga && !s.Finished && s.Status != "fail" {
		s.Status = "running"
	}
	if create != nil {
		if m, ok := create.PayloadOut.(map[string]any); ok {
			if id, ok := m["ID"].(float64); ok {
				s.JobID = &id
			}
		}
	}
	MaskSaga(s)
	return s, nil
}

type builder struct {
	apps      map[string]AppInfo
	events    []*Event
	steps     []*Step
	workerS   []*Step
	operation string
	org, user string
	edges     []Edge
}

// parseLine lê uma linha do docker: o evento já classificado, a aplicação (pela tag do log)
// e o transaction_id. ok=false para linha que não é log de container.
func parseLine(l RawLine) (e *Event, info AppInfo, tx string, ok bool) {
	var d dockerLine
	if err := json.Unmarshal([]byte(l.Line), &d); err != nil || d.Log == "" {
		return nil, AppInfo{}, "", false
	}
	tag := strings.Split(d.Attrs.Tag, "|")
	app := "desconhecido"
	if len(tag) > 1 && tag[1] != "" {
		app = tag[1]
	}
	image, version := tag[0], ""
	if i := strings.LastIndexByte(tag[0], ':'); i >= 0 {
		image, version = tag[0][:i], tag[0][i+1:]
	}
	info = appInfo(app, image, version)
	ts, okTS := dockerTS(d.Time)
	if !okTS {
		ts = float64(l.TS.UnixNano()) / 1e6
	}

	format, f := parseLog(d.Log)
	e = &Event{
		TS:                ts,
		App:               app,
		Format:            format,
		Level:             strings.ToLower(f.str("level")),
		Msg:               f.str("msg"),
		Action:            f.str("action"),
		Resource:          f.str("resource"),
		Integration:       f.str("integration"),
		IntegrationMethod: f.str("integration_method"),
		Raw:               strings.TrimSpace(d.Log),
		user:              f.str("user_id"),
	}
	if e.Msg == "" {
		e.Msg = strings.TrimSpace(f.str("package") + " " + f.str("method"))
	}
	if src, ok := f["source"].(map[string]any); ok {
		e.Source = fmt.Sprintf("%v:%v", src["file"], src["line"])
	} else {
		e.Source = f.str("source")
	}
	if b, ok := tryJSON(f["broker_msg_metadata"]).(map[string]any); ok {
		e.Broker = b
	}
	classify(app, info.Role, f, e)
	if e.Error == "" {
		// o detalhe do erro vem num campo à parte (core/v2: "err"); "Action execution failed" sozinho não diz nada
		e.Error = or(f.str("err"), f.str("error"))
	}
	return e, info, f.str("transaction_id"), true
}

func (b *builder) role(e *Event) Role { return b.apps[e.App].Role }

func (b *builder) newStep(app string, kind StepKind) *Step {
	s := &Step{
		ID:       fmt.Sprintf("s%d", len(b.steps)),
		App:      app,
		Kind:     kind,
		Label:    b.apps[app].Label,
		Start:    math.Inf(1),
		End:      math.Inf(-1),
		Status:   "ok",
		EventIDs: []string{},
		Calls:    []Call{},
		JobCalls: []Call{},
	}
	b.steps = append(b.steps, s)
	return s
}

func add(s *Step, e *Event) {
	s.EventIDs = append(s.EventIDs, e.ID)
	s.Start = math.Min(s.Start, e.TS)
	s.End = math.Max(s.End, e.TS)
	e.StepID = s.ID
}

func callOf(e *Event, typ, target string) Call {
	return Call{TS: e.TS, Type: typ, Target: target, Payload: e.Payload, EventID: e.ID, DurationMs: e.DurationMs, Status: ""}
}

// gateway: requisição HTTP + relay do outbox.
func (b *builder) gateway() {
	var gw, relay []*Event
	for _, e := range b.events {
		if b.role(e) == RoleGateway {
			if e.Kind == KindOutboxRelay {
				relay = append(relay, e)
			} else {
				gw = append(gw, e)
			}
		}
	}
	if len(gw) > 0 {
		s := b.newStep(gw[0].App, StepGateway)
		s.Label = "Requisição recebida"
		for _, e := range gw {
			add(s, e)
			switch e.Kind {
			case KindHTTPIn:
				s.Endpoint, s.PayloadIn = e.Target, e.Payload
				s.Action, s.Resource = strings.SplitN(e.Target, " ", 2)[0], lastPathSegment(e.Target)
				b.operation = operationName(e.Target)
				_, path, _ := strings.Cut(e.Target, " ")
				b.org, b.user = pathParam(path, "organizations"), e.user
			case KindHTTPOut:
				s.PayloadOut, s.HTTPStatus = e.Payload, e.Status
				// gateway respondeu 4xx/5xx ao cliente: a requisição falhou
				if n, err := strconv.Atoi(e.Status); err == nil && n >= 400 {
					s.Status = "fail"
				}
			case KindIntegrationCall, KindOutboxEnqueue:
				s.Calls = append(s.Calls, callOf(e, e.ResourceType, e.Target))
			}
		}
	}
	if len(relay) > 0 {
		s := b.newStep(relay[0].App, StepOutbox)
		s.Label = "Outbox → orquestrador"
		fails := 0
		for _, e := range relay {
			add(s, e)
			if e.Error != "" {
				fails++
			}
		}
		if fails > 0 {
			// entregou depois de N falhas (ou ainda não entregou): alerta, com as tentativas
			s.Status, s.Attempts = "warn", fails+1
		}
	}
}

// orchestrator: criação do job (primeiro POST + resposta + publish da primeira etapa).
func (b *builder) orchestrator() *Step {
	var post, resp, pub *Event
	for _, e := range b.events {
		if b.role(e) != RoleOrchestrator {
			continue
		}
		switch {
		case post == nil && e.Kind == KindJobRequest && strings.HasPrefix(e.Target, "POST"):
			post = e
		case post != nil && resp == nil && e.Kind == KindJobResponse:
			resp = e
		case pub == nil && e.Kind == KindPublish:
			pub = e
		}
	}
	if post == nil {
		return nil
	}
	s := b.newStep(post.App, StepOrchestrator)
	s.Label, s.Action, s.Resource = "Job criado", "create", lastPathSegment(post.Target)
	add(s, post)
	s.Endpoint, s.PayloadIn = post.Target, post.Payload
	if resp != nil {
		add(s, resp)
		s.PayloadOut, s.HTTPStatus = resp.Payload, resp.Status
	}
	if pub != nil {
		add(s, pub)
		s.Publish = &Publish{Exchange: pub.Exchange, Routing: pub.Routing}
		s.Calls = append(s.Calls, callOf(pub, "queue", pub.Target))
	}
	return s
}

// workers: uma etapa por execução; nova execução quando chega step_start depois de um step_end (gap > 100ms).
func (b *builder) workers() {
	byApp := map[string][]*Event{}
	var order []string
	for _, e := range b.events {
		if b.role(e) != RoleWorker {
			continue
		}
		if _, ok := byApp[e.App]; !ok {
			order = append(order, e.App)
		}
		byApp[e.App] = append(byApp[e.App], e)
	}
	for _, app := range order {
		var cur *Step
		ended, last := false, 0.0
		for _, e := range byApp[app] {
			if cur == nil || (e.Kind == KindStepStart && ended && e.TS-last > 100) {
				cur = b.newStep(app, StepWorker)
				b.workerS = append(b.workerS, cur)
				ended = false
			}
			add(cur, e)
			last = e.TS
			if cur.Action == "" {
				cur.Action = e.Action
			}
			if cur.Resource == "" {
				cur.Resource = e.Resource
			}
			switch e.Kind {
			case KindStepEnd:
				ended = true
				if e.Payload != nil {
					cur.PayloadOut = e.Payload
				}
			case KindStateIn, KindStepStart:
				if cur.PayloadIn == nil && e.Payload != nil {
					cur.PayloadIn = e.Payload
				}
			case KindIntegrationCall:
				cur.Calls = append(cur.Calls, callOf(e, e.Integration, e.IntegrationMethod))
			case KindPublish:
				if e.Routing != "" {
					cur.Publish = &Publish{Exchange: e.Exchange, Routing: e.Routing}
				}
				if PublishFailed(e.Status) {
					cur.Status = "fail"
				}
				if e.Broker != nil {
					cur.Broker = e.Broker
				}
				if e.Status != "" {
					cur.PublishStatus = e.Status
				}
			case KindError:
				// erro só logado não derruba a saga: vira alerta. Falha é o worker publicar ERROR.
				if cur.Status != "fail" {
					cur.Status = "warn"
				}
			}
		}
	}
}

// jobCalls: GET/PATCH do job no orquestrador vão para a etapa do worker que fez a chamada.
func (b *builder) jobCalls(create *Step) {
	var pending []*Event
	for _, e := range b.events {
		if b.role(e) == RoleOrchestrator && e.StepID == "" {
			pending = append(pending, e)
		}
	}
	for _, req := range pending {
		if req.Kind != KindJobRequest {
			continue
		}
		var resp *Event
		for _, e := range pending {
			if e.Kind == KindJobResponse && e.TS >= req.TS && e.StepID == "" {
				resp = e
				break
			}
		}
		// PATCH: etapa ativa (última que começou); GET: próxima etapa a começar
		var host *Step
		if strings.HasPrefix(req.Target, "PATCH") {
			for _, s := range b.workerS {
				if s.Start <= req.TS+1 && (host == nil || s.Start > host.Start) {
					host = s
				}
			}
		} else {
			for _, s := range b.workerS {
				if s.Start >= req.TS-5 && (host == nil || s.Start < host.Start) {
					host = s
				}
			}
		}
		target := host
		if target == nil {
			target = create
		}
		if target == nil {
			continue
		}
		c := Call{TS: req.TS, Type: "orchestrator-api", Target: req.Target, Payload: req.Payload, EventID: req.ID}
		if resp != nil {
			c.DurationMs, c.Status = resp.DurationMs, resp.Status
			if c.Payload == nil {
				c.Payload = resp.Payload
			}
			resp.StepID = target.ID
		}
		target.JobCalls = append(target.JobCalls, c)
		req.StepID = target.ID
		target.EventIDs = append(target.EventIDs, req.ID)
		if resp != nil {
			target.EventIDs = append(target.EventIDs, resp.ID)
		}
		// a etapa do worker começa quando ele pede o state do job
		if target != create {
			target.Start = math.Min(target.Start, req.TS)
			end := req.TS
			if resp != nil {
				end = resp.TS
			}
			target.End = math.Max(target.End, end)
		}
	}
}

var routingRe = regexp.MustCompile(`^v\d+\.(\d+)\.(.+)$`)

func (b *builder) finish() {
	byID := make(map[string]*Event, len(b.events))
	for _, e := range b.events {
		byID[e.ID] = e
	}
	// duração das chamadas = até o próximo evento da etapa, quando o log não informa
	for _, s := range b.steps {
		evs := make([]*Event, 0, len(s.EventIDs))
		for _, id := range s.EventIDs {
			evs = append(evs, byID[id])
		}
		slices.SortStableFunc(evs, func(a, b *Event) int { return cmpF(a.TS, b.TS) })
		for i := range s.Calls {
			c := &s.Calls[i]
			if c.DurationMs != nil {
				continue
			}
			d := 0.0
			for _, e := range evs {
				if e.TS > c.TS && e.ID != c.EventID {
					d = math.Round(e.TS - c.TS)
					break
				}
			}
			c.DurationMs = ptr(d)
		}
		s.DurationMs = math.Round(s.End - s.Start)
	}

	slices.SortStableFunc(b.steps, func(a, b *Step) int { return cmpF(a.Start, b.Start) })
	b.collapsePolling(byID)
	for i, s := range b.steps {
		s.Order = i
	}
	for i := 0; i+1 < len(b.steps); i++ {
		from, to := b.steps[i], b.steps[i+1]
		e := Edge{From: from.ID, To: to.ID, WaitMs: math.Max(0, math.Round(to.Start-from.End)), Via: "http"}
		switch {
		case from.Kind == StepGateway:
			e.Via = "outbox"
		case from.Publish != nil || from.Kind == StepWorker:
			e.Via = "queue"
		}
		if from.Publish != nil {
			e.Routing, e.Exchange = from.Publish.Routing, from.Publish.Exchange
			to.RoutingIn = from.Publish.Routing
			rest := strings.TrimPrefix(from.Publish.Routing, "v1.")
			if m := routingRe.FindStringSubmatch(from.Publish.Routing); m != nil {
				var n int
				fmt.Sscanf(m[1], "%d", &n)
				to.SagaIndex = &n
				rest = m[2]
			}
			// logrus não traz action/resource: deduz da routing key (v1.7.fortinet.vip.create → vip / create)
			if parts := strings.Split(rest, "."); to.Kind == StepWorker && len(parts) >= 2 {
				if to.Action == "" {
					to.Action = parts[len(parts)-1]
				}
				if to.Resource == "" {
					to.Resource = parts[len(parts)-2]
				}
			}
		}
		b.edges = append(b.edges, e)
	}

	// o core/v2 em JSON não loga a routing key: a saga é sequencial, então o índice é o anterior + 1
	for i := 1; i < len(b.steps); i++ {
		prev, s := b.steps[i-1], b.steps[i]
		if s.Kind == StepWorker && s.SagaIndex == nil && prev.SagaIndex != nil {
			n := *prev.SagaIndex + 1
			s.SagaIndex = &n
		}
	}

	count := map[string]int{}
	for _, s := range b.steps {
		count[s.App]++
		s.Execution = count[s.App]
	}
	for _, s := range b.steps {
		if s.Kind == StepWorker && count[s.App] > 1 {
			s.Label += fmt.Sprintf(" · execução %d", s.Execution)
		}
	}
}

// collapsePolling junta as iterações de um worker que republica na própria routing
// (ex.: v1.4.ontap.volume.status → v1.4.ontap.volume.status até o clone terminar).
func (b *builder) collapsePolling(byID map[string]*Event) {
	for i := 1; i < len(b.steps); i++ {
		if p := b.steps[i-1].Publish; p != nil {
			b.steps[i].RoutingIn = p.Routing
		}
	}
	out := b.steps[:0]
	for _, s := range b.steps {
		if n := len(out); n > 0 {
			g := out[n-1]
			loop := g.RoutingIn
			if g.Kind == StepWorker && s.Kind == StepWorker && g.App == s.App && loop != "" &&
				s.RoutingIn == loop && g.Publish != nil && g.Publish.Routing == loop {
				if g.Attempts == 0 {
					g.Attempts = 1
				}
				g.Attempts++
				g.EventIDs = append(g.EventIDs, s.EventIDs...)
				for _, id := range s.EventIDs {
					byID[id].StepID = g.ID
				}
				g.Calls = append(g.Calls, s.Calls...)
				g.JobCalls = append(g.JobCalls, s.JobCalls...)
				g.End = math.Max(g.End, s.End)
				g.DurationMs = math.Round(g.End - g.Start)
				g.Publish, g.PublishStatus, g.Broker = s.Publish, s.PublishStatus, s.Broker
				if s.PayloadOut != nil {
					g.PayloadOut = s.PayloadOut
				}
				if s.Status == "fail" || (s.Status == "warn" && g.Status == "ok") {
					g.Status = s.Status
				}
				continue
			}
		}
		out = append(out, s)
	}
	b.steps = out
}

var reID = regexp.MustCompile(`^([0-9]+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// lastPathSegment devolve a última coleção do caminho, pulando IDs:
// "PATCH /v1/organizations/<uuid>/nats/<uuid>" → nats; "GET /nats/489564" → nats.
func lastPathSegment(target string) string {
	path := target
	if i := strings.IndexByte(target, ' '); i >= 0 {
		path = target[i+1:]
	}
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" && !reID.MatchString(parts[i]) {
			return parts[i]
		}
	}
	return ""
}

var (
	verbOf = map[string]string{"POST": "Criar", "PUT": "Editar", "PATCH": "Editar", "DELETE": "Excluir", "GET": "Consultar"}
	nounOf = map[string]string{
		"nats":                    "NAT",
		"public-address":          "endereço público",
		"load-balances":           "load balance",
		"databases":               "banco de dados",
		"shares":                  "compartilhamento",
		"internal-load-balancers": "load balancer interno",
	}
)

// operationName: "PATCH /v1/.../nats/<id>" → "Editar NAT".
func operationName(target string) string {
	method, _, _ := strings.Cut(target, " ")
	res := lastPathSegment(target)
	noun, ok := nounOf[res]
	if !ok {
		noun = strings.ReplaceAll(res, "-", " ")
	}
	verb, ok := verbOf[strings.ToUpper(method)]
	if !ok {
		verb = method
	}
	return strings.TrimSpace(verb + " " + noun)
}

func cmpF(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// eventErrorText: a mensagem de erro de um evento de erro ou de publish com falha, completa.
func eventErrorText(e *Event) string {
	msg := strings.TrimSpace(e.Msg)
	if e.Error != "" && !strings.Contains(msg, e.Error) {
		if msg != "" {
			msg += ": "
		}
		msg += e.Error
	}
	if e.Kind == KindPublish && PublishFailed(e.Status) {
		// "publishing message" e "message published" trazem o mesmo erro: fica só o texto dele
		msg = PublishedPrefix + e.Status + ": " + or(e.Error, strings.TrimSpace(e.Msg))
	}
	return msg
}

// PublishedPrefix marca a mensagem de um publish com falha ("publicou ERROR: ...").
const PublishedPrefix = "publicou "

// collectErrors junta em cada etapa as mensagens de erro dos seus eventos, completas.
func collectErrors(s *Saga) {
	byID := map[string]*Step{}
	for _, st := range s.Steps {
		byID[st.ID] = st
	}
	for _, e := range s.Events {
		st := byID[e.StepID]
		if st == nil {
			continue
		}
		failedPublish := e.Kind == KindPublish && PublishFailed(e.Status)
		httpErr := e.Kind == KindHTTPOut && st.Kind == StepGateway && st.Status == "fail"
		relayErr := e.Kind == KindOutboxRelay && e.Error != ""
		if e.Kind != KindError && !failedPublish && !httpErr && !relayErr {
			continue
		}
		msg := eventErrorText(e)
		if httpErr {
			if v := httpErrorText(e.Payload); v != "" {
				msg = v
			}
			msg = "HTTP " + e.Status + ": " + msg
		}
		if msg == "" {
			msg = e.Raw
		}
		if slices.ContainsFunc(st.Errors, func(x StepError) bool { return x.Msg == msg }) {
			continue
		}
		st.Errors = append(st.Errors, StepError{TS: e.TS, EventID: e.ID, Msg: msg, Cause: ClassifyError(msg, e.App)})
	}
}

// goErrMsgRe: o texto de um views.Error logado com %+v ({Code:400 Message:... References:[] Accept:[]}).
var goErrMsgRe = regexp.MustCompile(`(?s)\bMessage:(.*?)(?:\s+References:\[|\s+Accept:\[|\}\s*$)`)

// httpErrorText: a mensagem da resposta de erro do gateway. O campo message vem em JSON ({"message": ...}) ou como
// struct do Go impressa com %+v; nesse caso fica só o Message.
func httpErrorText(payload any) string {
	switch p := payload.(type) {
	case map[string]any:
		if v, ok := p["message"].(string); ok {
			return strings.TrimSpace(v)
		}
	case string:
		if m := goErrMsgRe.FindStringSubmatch(p); m != nil {
			return strings.TrimSpace(m[1])
		}
		return strings.TrimSpace(p)
	}
	return ""
}

// PublishFailed: o worker publicou uma falha. IN_PROGRESS é normal (ainda trabalhando, ex.: polling).
func PublishFailed(status string) bool {
	return status != "" && status != "COMPLETED" && status != "IN_PROGRESS"
}

// sagaStatus: ok | fail | recovered (running é decidido no Build, que sabe se houve worker final).
//   - fail: um worker publicou ERROR, ou o gateway respondeu 4xx/5xx (sem saga). Só isso é falha.
//   - recovered: alguma etapa logou erro (warn), mas ninguém publicou ERROR.
func sagaStatus(steps []*Step) string {
	warned := false
	for _, st := range steps {
		if PublishFailed(st.PublishStatus) || (st.Kind == StepGateway && st.Status == "fail") {
			return "fail"
		}
		if st.Status == "warn" || st.Status == "fail" {
			warned = true
		}
	}
	if warned {
		return "recovered"
	}
	return "ok"
}

// sem padrão: só label sagalogs.final=true ou SAGALOGS_FINAL_APPS marcam o fim da saga
var finalAppsRe *regexp.Regexp

// SetFinalApps troca a regex dos workers que fecham a saga (SAGALOGS_FINAL_APPS).
func SetFinalApps(pattern string) error {
	if pattern == "" {
		finalAppsRe = nil
		return nil
	}
	re, err := regexp.Compile("^(" + pattern + ")$")
	if err != nil {
		return err
	}
	finalAppsRe = re
	return nil
}

func finalConfigured() bool {
	if finalAppsRe != nil {
		return true
	}
	reg.RLock()
	defer reg.RUnlock()
	for _, o := range reg.over {
		if o.final {
			return true
		}
	}
	return false
}

// IsFinalApp diz se o container grava o resultado final da saga.
func IsFinalApp(app string) bool {
	reg.RLock()
	f := reg.over[app].final
	reg.RUnlock()
	return f || (finalAppsRe != nil && finalAppsRe.MatchString(app))
}

// IsGatewayApp diz se o container é o gateway (recebe as ações e cria as transações).
func IsGatewayApp(app string) bool { return appInfo(app, "", "").Role == RoleGateway }
