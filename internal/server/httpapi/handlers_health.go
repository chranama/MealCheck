package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/access"
	"github.com/chranama/MealCheck/internal/runs/submission"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	stats, err := s.Store.Stats(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_unavailable", "store unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                      "ok",
		"store":                       s.Config.StoreKind,
		"access_mode":                 access.Mode(s.Config),
		"hosted_mode":                 submission.HostedMode(s.Config),
		"queued_runs":                 stats.Queued,
		"running_runs":                stats.Running,
		"queue_size":                  s.Config.QueueSize,
		"active_run_limit":            1,
		"retention_days":              int(s.Config.Retention.Hours() / 24),
		"public_openai_compatible":    s.Config.PublicOpenAICompatible,
		"max_candidate_text_chars":    s.Config.MaxCandidateTextChars,
		"max_generation_prompt_chars": s.Config.MaxGenerationPromptChars,
		"local_model":                 s.localModelHealth(r.Context()),
		"policy": map[string]any{
			"public_request_limit":      s.Config.PublicRequestLimit,
			"public_request_window_sec": int(s.Config.PublicRequestWindow.Seconds()),
			"public_daily_run_limit":    s.Config.PublicDailyRunLimit,
			"queue_size":                s.Config.QueueSize,
			"active_run_limit":          1,
		},
	})
}
func (s *Server) localModelHealth(ctx context.Context) map[string]any {
	modelName := strings.TrimSpace(s.Config.LocalModelName)
	if modelName != "" {
		modelName = filepath.Base(modelName)
	}
	status := map[string]any{
		"enabled":                 s.Config.LocalModelEnabled,
		"ready":                   false,
		"model":                   modelName,
		"max_input_chars":         s.Config.LocalModelMaxInputChars,
		"max_source_items":        s.Config.LocalModelMaxSourceItems,
		"max_output_tokens":       s.Config.LocalModelMaxOutputTokens,
		"timeout_sec":             int(s.Config.LocalModelTimeout.Seconds()),
		"supported_days":          1,
		"supported_meals_per_day": 6,
	}
	if !s.Config.LocalModelEnabled {
		return status
	}
	if err := submission.ValidateLocalModelAvailable(s.Config); err != nil {
		status["error"] = err.Error()
		return status
	}
	baseURL := strings.TrimRight(strings.TrimSpace(s.Config.LocalModelBaseURL), "/")
	readyCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(readyCtx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		status["error"] = "local model readiness request could not be built"
		return status
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		status["error"] = "local model endpoint is not reachable"
		return status
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		status["ready"] = true
		return status
	}
	status["error"] = fmt.Sprintf("local model endpoint returned HTTP %d", resp.StatusCode)
	return status
}
