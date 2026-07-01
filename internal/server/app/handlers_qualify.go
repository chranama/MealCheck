package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) handleQualify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if _, err := s.authorizeRun(r); errors.Is(err, ErrInviteRunLimit) {
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
	var request MealPlanQualificationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON request", nil)
		return
	}
	providerSupplied := hasProviderConfigFields(request.Provider)
	request.Text = strings.TrimSpace(request.Text)
	request.Provider = normalizeProviderConfig(request.Provider)
	if request.Text == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "text is required", nil)
		return
	}
	textLimit := s.Config.MaxCandidateTextChars
	if hostedMode(s.Config) == HostedModeLocalModel {
		textLimit = s.Config.LocalModelMaxInputChars
	}
	if err := validateTextLength("text", request.Text, textLimit); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if hostedMode(s.Config) == HostedModeLocalModel {
		request.Settings = normalizeLocalModelSettings(request.Settings)
	}
	if err := validateSettings(request.Settings); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if hostedMode(s.Config) == HostedModeLocalModel {
		if providerSupplied {
			writeError(w, r, http.StatusBadRequest, "invalid_request", "hosted local model mode does not accept client provider configuration", nil)
			return
		}
		if err := validateLocalModelAvailable(s.Config); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "local_model_unavailable", err.Error(), nil)
			return
		}
		if err := validateLocalModelSettings(request.Settings); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		if err := validateLocalModelMealPlanPreflight(s.Config, request.Text); err != nil {
			if writeMealPlanNotVerifiableError(w, r, err) {
				return
			}
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		request.Provider = localModelProviderConfig(s.Config)
	} else if providerSupplied {
		if err := validatePublicProviderPolicy(s.Config, request.Provider); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
	}

	providerFactory := s.ProviderFactory
	if providerFactory == nil {
		providerFactory = DefaultProviderFactory
	}
	qualification, err := QualifyMealPlanText(r.Context(), providerFactory, request)
	if err != nil {
		if isProviderConfigError(err) {
			writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		writeError(w, r, http.StatusBadGateway, "provider_error", sanitizeProviderErrorText(err.Error(), request.Provider.APIKey), nil)
		return
	}
	writeJSON(w, http.StatusOK, QualifyMealPlanResponse{Qualification: qualification})
}
