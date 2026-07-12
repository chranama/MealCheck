package access

import (
	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
	"github.com/chranama/MealCheck/internal/state"
)

type Config = core.Config
type InviteToken = core.InviteToken
type ProviderConfig = core.ProviderConfig

const (
	ProviderTypeOpenAICompatible = inference.ProviderTypeOpenAICompatible
	ProviderTypeLocalLlama       = inference.ProviderTypeLocalLlama

	AccessModePublicBYOK     = core.AccessModePublicBYOK
	AccessModeInviteRequired = core.AccessModeInviteRequired
)

var (
	ErrInviteUnavailable = state.ErrInviteUnavailable
	ErrInviteRunLimit    = state.ErrInviteRunLimit
)
