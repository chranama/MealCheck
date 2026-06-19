package hosted

import (
	"encoding/json"
	"time"

	"github.com/chranama/MealCheck/internal/checker"
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
	EventPlanNormalized  = "plan_normalized"
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
	InviteRequired   bool
	QueueSize        int
	MaxCasesPerRun   int
	MaxUploadBytes   int64
	RunTimeout       time.Duration
	PendingInputTTL  time.Duration
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

type InviteToken struct {
	ID         string     `json:"id"`
	SecretHash string     `json:"-"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	MaxRuns    *int       `json:"max_runs,omitempty"`
	UsedRuns   int        `json:"used_runs"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type CreateRunRequest struct {
	CasePath         string              `json:"case_path,omitempty"`
	InputMode        string              `json:"input_mode,omitempty"`
	Profile          checker.Profile     `json:"profile,omitempty"`
	Constraints      checker.Constraints `json:"constraints,omitempty"`
	CandidatePlan    *checker.Plan       `json:"candidate_plan,omitempty"`
	GenerationPrompt string              `json:"generation_prompt,omitempty"`
	Provider         ProviderConfig      `json:"provider,omitempty"`
	RepairJSON       *bool               `json:"repair_json,omitempty"`
}

type QualifyMealPlanResponse struct {
	Qualification MealPlanQualificationResult `json:"qualification"`
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

type ProviderConfig struct {
	Type    string `json:"type,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Model   string `json:"model,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
}

type RedactedProviderConfig struct {
	Type    string `json:"type"`
	BaseURL string `json:"base_url,omitempty"`
	Model   string `json:"model,omitempty"`
	APIKey  string `json:"api_key"`
}

type PendingRunInput struct {
	Mode             string
	Profile          checker.Profile
	Constraints      checker.Constraints
	CandidatePlan    *checker.Plan
	GenerationPrompt string
	Provider         ProviderConfig
	RepairJSON       bool
}

func jsonRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
