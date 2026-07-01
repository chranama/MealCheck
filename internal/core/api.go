package core

import (
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

type CreateRunRequest struct {
	CasePath         string           `json:"case_path,omitempty"`
	InputMode        string           `json:"input_mode,omitempty"`
	Settings         checker.Settings `json:"settings,omitempty"`
	CandidatePlan    *checker.Plan    `json:"candidate_plan,omitempty"`
	CandidateText    string           `json:"candidate_text,omitempty"`
	GenerationPrompt string           `json:"generation_prompt,omitempty"`
	Provider         ProviderConfig   `json:"provider,omitempty"`
	RepairJSON       *bool            `json:"repair_json,omitempty"`
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

type RunDocument struct {
	Run      Run          `json:"run"`
	Links    RunLinks     `json:"links"`
	Progress *RunProgress `json:"progress,omitempty"`
}

type RunProgress struct {
	State      string          `json:"state"`
	Label      string          `json:"label"`
	Message    string          `json:"message"`
	LastEvent  string          `json:"last_event,omitempty"`
	Recovery   *RecoveryNotice `json:"recovery,omitempty"`
	UpdatedAt  time.Time       `json:"updated_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

type RecoveryNotice struct {
	Title   string          `json:"title"`
	Message string          `json:"message"`
	Tone    string          `json:"tone"`
	Steps   []string        `json:"steps,omitempty"`
	Action  *RecoveryAction `json:"action,omitempty"`
}

type RecoveryAction struct {
	Label string `json:"label"`
	Href  string `json:"href"`
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

type PublicStatusResponse struct {
	SchemaVersion   string            `json:"schema_version"`
	GeneratedAt     time.Time         `json:"generated_at"`
	Overall         StatusSummary     `json:"overall"`
	Components      []StatusComponent `json:"components"`
	RecentIncidents []StatusIncident  `json:"recent_incidents"`
	Links           StatusLinks       `json:"links,omitempty"`
}

type StatusSummary struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type StatusComponent struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type StatusIncident struct {
	ID         string         `json:"id"`
	Title      string         `json:"title"`
	State      string         `json:"state"`
	StartedAt  time.Time      `json:"started_at"`
	ResolvedAt *time.Time     `json:"resolved_at,omitempty"`
	Updates    []StatusUpdate `json:"updates,omitempty"`
}

type StatusUpdate struct {
	State     string    `json:"state"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type StatusLinks struct {
	SampleReport string `json:"sample_report,omitempty"`
}

type PendingRunInput struct {
	Mode             string
	Settings         checker.Settings
	CandidatePlan    *checker.Plan
	CandidateText    string
	GenerationPrompt string
	Provider         ProviderConfig
	RepairJSON       bool
}
