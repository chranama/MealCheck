package core

import "time"

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
