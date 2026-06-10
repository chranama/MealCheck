package hosted

import (
	"encoding/json"
	"time"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusDeleted   = "deleted"
)

const (
	EventQueued          = "queued"
	EventStarted         = "started"
	EventArtifactWritten = "artifact_written"
	EventCompleted       = "completed"
	EventFailed          = "failed"
)

type Config struct {
	Root             string
	DataDir          string
	ArtifactDir      string
	Addr             string
	DatabaseURL      string
	StoreKind        string
	AllowedOrigin    string
	InviteToken      string
	QueueSize        int
	MaxCasesPerRun   int
	MaxUploadBytes   int64
	RunTimeout       time.Duration
	Retention        time.Duration
	WorkerPoll       time.Duration
	CleanupInterval  time.Duration
	DemoIndexPath    string
	DemoArtifactRoot string
}

type Run struct {
	ID          string     `json:"id"`
	CasePath    string     `json:"case_path"`
	Status      string     `json:"status"`
	Decision    string     `json:"decision,omitempty"`
	RiskLevel   string     `json:"risk_level,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	ArtifactDir string     `json:"-"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

type RunEvent struct {
	ID        int64     `json:"id"`
	RunID     string    `json:"run_id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateRunRequest struct {
	CasePath string `json:"case_path"`
}

type CreateRunResponse struct {
	RunID     string    `json:"run_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	Links     RunLinks  `json:"links"`
}

type RunLinks struct {
	Self      string `json:"self"`
	Events    string `json:"events"`
	Report    string `json:"report"`
	Artifacts string `json:"artifacts"`
}

type ArtifactList struct {
	RunID     string             `json:"run_id"`
	Artifacts []ArtifactListItem `json:"artifacts"`
}

type ArtifactListItem struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	Type string `json:"type"`
}

type DemoIndex struct {
	SchemaVersion string    `json:"schema_version"`
	DemoRuns      []DemoRun `json:"demo_runs"`
}

type DemoRun struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	BasePath string `json:"base_path"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Details   map[string]any `json:"details,omitempty"`
}

func jsonRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
