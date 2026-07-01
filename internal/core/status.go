package core

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

const (
	AccessModePublicBYOK     = "public_byok"
	AccessModeInviteRequired = "invite_required"
)

const (
	HostedModeBYOK       = "byok"
	HostedModeLocalModel = "local_model"
)

const (
	InputModeManualStructured  = "manual_structured"
	InputModeProfileGeneration = "profile_generation"
	InputModePromptGeneration  = "prompt_generation"
	InputModeLocalModel        = "local_model"
)

const (
	StatusStateOperational   = "operational"
	StatusStateDegraded      = "degraded_performance"
	StatusStatePartialOutage = "partial_outage"
	StatusStateMajorOutage   = "major_outage"
	StatusStateMaintenance   = "maintenance"
	StatusStateUnknown       = "unknown"
)
