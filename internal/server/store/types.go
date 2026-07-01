package store

import "github.com/chranama/MealCheck/internal/core"

type Config = core.Config
type Run = core.Run
type RunEvent = core.RunEvent
type InviteToken = core.InviteToken

const (
	StatusQueued    = core.StatusQueued
	StatusRunning   = core.StatusRunning
	StatusCompleted = core.StatusCompleted
	StatusFailed    = core.StatusFailed
	StatusDeleted   = core.StatusDeleted
)
