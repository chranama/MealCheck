package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/chranama/MealCheck/internal/access"
	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
	"github.com/chranama/MealCheck/internal/llm/planextract"
	"github.com/chranama/MealCheck/internal/runs/submission"
	"github.com/chranama/MealCheck/internal/state"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	"github.com/chranama/MealCheck/internal/workflow/normalize"
)

func (s *Server) handleQualify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if _, err := s.authorizeRun(r); errors.Is(err, state.ErrInviteRunLimit) {
		writeError(w, r, http.StatusTooManyRequests, "invite_limit_reached", "access code run limit reached", nil)
		return
	} else if err != nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "valid access code required", nil)
		return
	}
	if err := s.enforcePublicRequestPolicy(w, r); err != nil {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.Config.MaxUploadBytes)
	var request core.MealPlanQualificationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request", nil)
		return
	}
	providerSupplied := submission.HasProviderConfigFields(request.Provider)
	request.Text = strings.TrimSpace(request.Text)
	request.Provider = submission.NormalizeProviderConfig(request.Provider)
	if request.Text == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "text is required", nil)
		return
	}
	textLimit := s.Config.MaxCandidateTextChars
	if submission.HostedMode(s.Config) == core.HostedModeLocalModel {
		textLimit = s.Config.LocalModelMaxInputChars
	}
	if err := submission.ValidateTextLength("text", request.Text, textLimit); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if submission.HostedMode(s.Config) == core.HostedModeLocalModel {
		request.Settings = planextract.NormalizeLocalModelSettings(request.Settings)
	}
	if err := checker.ValidateSettings(request.Settings); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if submission.HostedMode(s.Config) == core.HostedModeLocalModel {
		if providerSupplied {
			writeError(w, r, http.StatusBadRequest, "invalid_request", "hosted local model mode does not accept client provider configuration", nil)
			return
		}
		if err := submission.ValidateLocalModelAvailable(s.Config); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "local_model_unavailable", err.Error(), nil)
			return
		}
		if err := submission.ValidateLocalModelSettings(request.Settings); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if err := planextract.ValidateLocalModelMealPlanPreflight(s.Config, request.Text); err != nil {
			if writeMealPlanNotVerifiableError(w, r, err) {
				return
			}
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		request.Provider = submission.LocalModelProviderConfig(s.Config)
	} else if providerSupplied {
		if err := access.ValidatePublicProviderPolicy(s.Config, request.Provider); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
	}

	completerFactory := s.CompleterFactory
	qualification, err := normalize.QualifyMealPlanText(r.Context(), completerFactory, request)
	if err != nil {
		if submission.IsProviderConfigError(err) {
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		writeError(w, r, http.StatusBadGateway, "provider_error", inference.SanitizeErrorText(err.Error(), request.Provider.APIKey), nil)
		return
	}
	writeJSON(w, http.StatusOK, core.QualifyMealPlanResponse{Qualification: qualification})
}
