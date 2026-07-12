package app

import (
	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
	"github.com/chranama/MealCheck/internal/server/access"
	"github.com/chranama/MealCheck/internal/state"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

type Config = core.Config
type Run = core.Run
type RunEvent = core.RunEvent
type InviteToken = core.InviteToken
type CreateRunRequest = core.CreateRunRequest
type CreateRunResponse = core.CreateRunResponse
type RunLinks = core.RunLinks
type RunDocument = core.RunDocument
type RunProgress = core.RunProgress
type RecoveryNotice = core.RecoveryNotice
type ArtifactList = core.ArtifactList
type ArtifactListItem = core.ArtifactListItem
type DemoIndex = core.DemoIndex
type DemoRun = core.DemoRun
type ErrorResponse = core.ErrorResponse
type APIError = core.APIError
type ProviderConfig = core.ProviderConfig
type RedactedProviderConfig = core.RedactedProviderConfig
type PublicStatusResponse = core.PublicStatusResponse
type StatusSummary = core.StatusSummary
type StatusComponent = core.StatusComponent
type StatusIncident = core.StatusIncident
type StatusUpdate = core.StatusUpdate
type StatusLinks = core.StatusLinks
type PendingRunInput = core.PendingRunInput
type Store = state.Store
type StoreStats = state.StoreStats
type PolicyLimiter = access.PolicyLimiter
type PolicyError = access.PolicyError
type CompleterFactory = inference.CompleterFactory
type Completer = inference.Completer
type MealPlanQualificationRequest = core.MealPlanQualificationRequest
type MealPlanQualificationResult = core.MealPlanQualificationResult
type QualifyMealPlanResponse = core.QualifyMealPlanResponse
type NormalizationEvent = normalize.NormalizationEvent

const (
	StatusQueued         = core.StatusQueued
	StatusRunning        = core.StatusRunning
	StatusAwaitingReview = core.StatusAwaitingReview
	StatusCompleted      = core.StatusCompleted
	StatusFailed         = core.StatusFailed
	StatusDeleted        = core.StatusDeleted

	EventQueued          = core.EventQueued
	EventStarted         = core.EventStarted
	EventPlanNormalized  = core.EventPlanNormalized
	EventReviewReady     = core.EventReviewReady
	EventReviewConfirmed = core.EventReviewConfirmed
	EventReviewCorrected = core.EventReviewCorrected
	EventReviewRejected  = core.EventReviewRejected
	EventReviewRewrite   = core.EventReviewRewrite
	EventArtifactWritten = core.EventArtifactWritten
	EventCompleted       = core.EventCompleted
	EventFailed          = core.EventFailed

	AccessModePublicBYOK     = core.AccessModePublicBYOK
	AccessModeInviteRequired = core.AccessModeInviteRequired

	HostedModeBYOK       = core.HostedModeBYOK
	HostedModeLocalModel = core.HostedModeLocalModel

	InputModeManualStructured  = core.InputModeManualStructured
	InputModeProfileGeneration = core.InputModeProfileGeneration
	InputModePromptGeneration  = core.InputModePromptGeneration
	InputModeLocalModel        = core.InputModeLocalModel

	ProviderTypeOpenAICompatible = inference.ProviderTypeOpenAICompatible
	ProviderTypeOpenAI           = inference.ProviderTypeOpenAI
	ProviderTypeAnthropic        = inference.ProviderTypeAnthropic
	ProviderTypeGemini           = inference.ProviderTypeGemini
	ProviderTypeLocalLlama       = inference.ProviderTypeLocalLlama

	StatusStateOperational   = core.StatusStateOperational
	StatusStateDegraded      = core.StatusStateDegraded
	StatusStatePartialOutage = core.StatusStatePartialOutage
	StatusStateMajorOutage   = core.StatusStateMajorOutage
	StatusStateMaintenance   = core.StatusStateMaintenance
	StatusStateUnknown       = core.StatusStateUnknown
)

var (
	ErrQueueFull         = state.ErrQueueFull
	ErrNotFound          = state.ErrNotFound
	ErrConflict          = state.ErrConflict
	ErrInviteUnavailable = state.ErrInviteUnavailable
	ErrInviteRunLimit    = state.ErrInviteRunLimit
)
