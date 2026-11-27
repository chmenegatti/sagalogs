// Package saga transforma linhas de log dos containers em uma saga navegável.
// O JSON produzido é o contrato consumido pela UI (ui/src/lib/saga-types.ts).
package saga

type EventKind string

const (
	KindHTTPIn          EventKind = "http_in"
	KindHTTPOut         EventKind = "http_out"
	KindOutboxEnqueue   EventKind = "outbox_enqueue"
	KindOutboxRelay     EventKind = "outbox_relay"
	KindJobRequest      EventKind = "job_request"
	KindJobResponse     EventKind = "job_response"
	KindStepStart       EventKind = "step_start"
	KindStateIn         EventKind = "state_in"
	KindStepEnd         EventKind = "step_end"
	KindPublish         EventKind = "publish"
	KindIntegrationCall EventKind = "integration_call"
	KindError           EventKind = "error"
	KindLog             EventKind = "log" // linha com msg que não é marco da saga (log de negócio do app)
	KindUnknown         EventKind = "unknown"
)

type Role string

const (
	RoleGateway      Role = "gateway"
	RoleOrchestrator Role = "orchestrator"
	RoleWorker       Role = "worker"
)

type AppInfo struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Role    Role   `json:"role"`
	Vendor  string `json:"vendor"`
	Final   bool   `json:"final,omitempty"` // worker que fecha a saga
	Image   string `json:"image,omitempty"`
	Version string `json:"version,omitempty"`
}

type Event struct {
	ID                string         `json:"id"`
	TS                float64        `json:"ts"` // epoch em ms, com fração
	App               string         `json:"app"`
	Format            string         `json:"format"` // json | logrus | slog
	Kind              EventKind      `json:"kind"`
	Level             string         `json:"level"`
	Msg               string         `json:"msg"`
	Action            string         `json:"action,omitempty"`
	Resource          string         `json:"resource,omitempty"`
	Integration       string         `json:"integration,omitempty"`
	IntegrationMethod string         `json:"integrationMethod,omitempty"`
	ResourceType      string         `json:"resourceType,omitempty"`
	Target            string         `json:"target,omitempty"`
	Exchange          string         `json:"exchange,omitempty"`
	Routing           string         `json:"routing,omitempty"`
	Status            string         `json:"status,omitempty"`
	Error             string         `json:"error,omitempty"`
	DurationMs        *float64       `json:"durationMs,omitempty"`
	Source            string         `json:"source,omitempty"`
	Broker            map[string]any `json:"broker,omitempty"`
	Payload           any            `json:"payload,omitempty"`
	Raw               string         `json:"raw"`
	StepID            string         `json:"stepId,omitempty"`
	user              string         // user_id (gateway); vai para Saga.User
}

// StepError é um erro logado na etapa, com a mensagem completa (msg + campo de erro).
type StepError struct {
	TS      float64    `json:"ts"`
	EventID string     `json:"eventId"`
	Msg     string     `json:"msg"`
	Cause   ErrorCause `json:"cause"`
}

type Call struct {
	TS         float64  `json:"ts"`
	Type       string   `json:"type"`
	Target     string   `json:"target"`
	Payload    any      `json:"payload,omitempty"`
	EventID    string   `json:"eventId"`
	DurationMs *float64 `json:"durationMs,omitempty"`
	Status     string   `json:"status,omitempty"`
}

type Publish struct {
	Exchange string `json:"exchange"`
	Routing  string `json:"routing"`
}

type StepKind string

const (
	StepGateway      StepKind = "gateway"
	StepOutbox       StepKind = "outbox"
	StepOrchestrator StepKind = "orchestrator"
	StepWorker       StepKind = "worker"
)

type Step struct {
	ID            string         `json:"id"`
	Order         int            `json:"order"`
	App           string         `json:"app"`
	Kind          StepKind       `json:"kind"`
	Label         string         `json:"label"`
	Action        string         `json:"action,omitempty"`
	Resource      string         `json:"resource,omitempty"`
	Start         float64        `json:"start"`
	End           float64        `json:"end"`
	DurationMs    float64        `json:"durationMs"`
	Status        string         `json:"status"` // ok | warn (logou erro, não publicou) | fail (publicou ERROR / HTTP 4xx-5xx)
	Errors        []StepError    `json:"errors,omitempty"`
	PublishStatus string         `json:"publishStatus,omitempty"`
	HTTPStatus    string         `json:"httpStatus,omitempty"`
	Endpoint      string         `json:"endpoint,omitempty"`
	SagaIndex     *int           `json:"sagaIndex,omitempty"`
	RoutingIn     string         `json:"routingIn,omitempty"`
	Publish       *Publish       `json:"publish,omitempty"`
	Broker        map[string]any `json:"broker,omitempty"`
	Execution     int            `json:"execution,omitempty"`
	Attempts      int            `json:"attempts,omitempty"` // >1 quando o worker ficou em loop de polling (publica na própria routing)
	PayloadIn     any            `json:"payloadIn,omitempty"`
	PayloadOut    any            `json:"payloadOut,omitempty"`
	EventIDs      []string       `json:"eventIds"`
	Calls         []Call         `json:"calls"`
	JobCalls      []Call         `json:"jobCalls"`
}

type Edge struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	WaitMs   float64 `json:"waitMs"`
	Routing  string  `json:"routing,omitempty"`
	Exchange string  `json:"exchange,omitempty"`
	Via      string  `json:"via"` // outbox | queue | http
}

type Saga struct {
	TransactionID string  `json:"transactionId"`
	CreatedAt     string  `json:"createdAt,omitempty"`
	Start         float64 `json:"start"`
	End           float64 `json:"end"`
	DurationMs    float64 `json:"durationMs"`
	Status        string  `json:"status"`
	// Finished: um worker final (label sagalogs.final ou SAGALOGS_FINAL_APPS) processou a saga.
	// Toda saga completa, com sucesso ou falha, termina gravando o resultado num deles.
	Finished  bool      `json:"finished"`
	Operation string    `json:"operation"`
	Org       string    `json:"org,omitempty"`  // organização do caminho da requisição
	User      string    `json:"user,omitempty"` // user_id do gateway
	JobID     *float64  `json:"jobId,omitempty"`
	Truncated bool      `json:"truncated,omitempty"` // ainda havia logs no fim da janela máxima
	Apps      []AppInfo `json:"apps"`
	Steps     []*Step   `json:"steps"`
	Edges     []Edge    `json:"edges"`
	Events    []*Event  `json:"events"`
}

// Summary é a linha da lista de sagas.
type Summary struct {
	TransactionID string  `json:"transactionId"`
	Operation     string  `json:"operation"`
	Status        string  `json:"status"`
	DurationMs    float64 `json:"durationMs"`
	Start         float64 `json:"start"`
	Steps         int     `json:"steps"`
	Apps          int     `json:"apps"`
}

func (s *Saga) Summary() Summary {
	return Summary{
		TransactionID: s.TransactionID,
		Operation:     s.Operation,
		Status:        s.Status,
		DurationMs:    s.DurationMs,
		Start:         s.Start,
		Steps:         len(s.Steps),
		Apps:          len(s.Apps),
	}
}
