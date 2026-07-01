package access

import (
	"github.com/chranama/MealCheck/internal/core"
	llm "github.com/chranama/MealCheck/internal/llm/external"
	"github.com/chranama/MealCheck/internal/server/store"
)

type Config = core.Config
type InviteToken = core.InviteToken
type ProviderConfig = core.ProviderConfig

const (
	ProviderTypeOpenAICompatible = llm.ProviderTypeOpenAICompatible
	ProviderTypeLocalLlama       = llm.ProviderTypeLocalLlama

	AccessModePublicBYOK     = core.AccessModePublicBYOK
	AccessModeInviteRequired = core.AccessModeInviteRequired
)

var (
	ErrInviteUnavailable = store.ErrInviteUnavailable
	ErrInviteRunLimit    = store.ErrInviteRunLimit
)
