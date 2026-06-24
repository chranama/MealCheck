package hosted

import (
	"context"
	"net/http"
	"path/filepath"
	"time"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	writeJSON(w, http.StatusOK, s.publicStatus(r.Context(), time.Now().UTC()))
}

func (s *Server) publicStatus(ctx context.Context, generatedAt time.Time) PublicStatusResponse {
	stats, storeErr := s.Store.Stats(ctx)
	queueAtCapacity := storeErr == nil && s.Config.QueueSize > 0 && stats.Queued >= s.Config.QueueSize
	localModelReady := s.publicLocalModelReady(ctx)
	sampleReportLink, sampleReportOK := s.sampleReportLink()

	components := []StatusComponent{
		mealCheckSubmissionStatus(storeErr, queueAtCapacity),
		aiMealNormalizationStatus(s.Config, storeErr, localModelReady),
		nutritionCheckingStatus(storeErr),
		reportGenerationStatus(s.Config, storeErr, queueAtCapacity, localModelReady),
		sampleReportStatus(sampleReportOK),
	}

	links := StatusLinks{}
	if sampleReportOK {
		links.SampleReport = sampleReportLink
	}

	return PublicStatusResponse{
		SchemaVersion:   "0.1",
		GeneratedAt:     generatedAt,
		Overall:         overallStatus(components),
		Components:      components,
		RecentIncidents: []StatusIncident{},
		Links:           links,
	}
}

func mealCheckSubmissionStatus(storeErr error, queueAtCapacity bool) StatusComponent {
	component := StatusComponent{ID: "meal_check_submission", Name: "Meal Check Submission"}
	switch {
	case storeErr != nil:
		component.State = StatusStateMajorOutage
		component.Message = "Meal checks cannot be accepted right now."
	case queueAtCapacity:
		component.State = StatusStateDegraded
		component.Message = "Meal checks are temporarily at capacity; retry shortly."
	default:
		component.State = StatusStateOperational
	}
	return component
}

func aiMealNormalizationStatus(config Config, storeErr error, localModelReady bool) StatusComponent {
	component := StatusComponent{ID: "ai_meal_normalization", Name: "AI Meal Normalization"}
	switch {
	case storeErr != nil:
		component.State = StatusStateMajorOutage
		component.Message = "AI meal normalization is unavailable while meal checks are offline."
	case hostedMode(config) == HostedModeLocalModel && !localModelReady:
		component.State = StatusStatePartialOutage
		component.Message = "Server-owned AI meal normalization is currently unavailable."
	case hostedMode(config) == HostedModeBYOK || hostedMode(config) == HostedModeLocalModel:
		component.State = StatusStateOperational
	default:
		component.State = StatusStateUnknown
		component.Message = "AI meal normalization status is unknown."
	}
	return component
}

func nutritionCheckingStatus(storeErr error) StatusComponent {
	component := StatusComponent{ID: "nutrition_allergen_checking", Name: "Nutrition & Allergen Checking"}
	if storeErr != nil {
		component.State = StatusStateMajorOutage
		component.Message = "Nutrition and allergen checks cannot complete while meal checks are offline."
		return component
	}
	component.State = StatusStateOperational
	return component
}

func reportGenerationStatus(config Config, storeErr error, queueAtCapacity bool, localModelReady bool) StatusComponent {
	component := StatusComponent{ID: "report_generation", Name: "Report Generation"}
	switch {
	case storeErr != nil:
		component.State = StatusStateMajorOutage
		component.Message = "Reports cannot be generated right now."
	case queueAtCapacity:
		component.State = StatusStateDegraded
		component.Message = "Reports may be delayed because the meal-check queue is at capacity."
	case hostedMode(config) == HostedModeLocalModel && !localModelReady:
		component.State = StatusStatePartialOutage
		component.Message = "Reports that need AI meal normalization may fail until the model is available."
	default:
		component.State = StatusStateOperational
	}
	return component
}

func sampleReportStatus(ok bool) StatusComponent {
	component := StatusComponent{ID: "sample_report", Name: "Sample Report"}
	if !ok {
		component.State = StatusStatePartialOutage
		component.Message = "The sample report is temporarily unavailable."
		return component
	}
	component.State = StatusStateOperational
	return component
}

func (s *Server) publicLocalModelReady(ctx context.Context) bool {
	if hostedMode(s.Config) != HostedModeLocalModel {
		return true
	}
	health := s.localModelHealth(ctx)
	ready, _ := health["ready"].(bool)
	return ready
}

func (s *Server) sampleReportLink() (string, bool) {
	index, err := s.loadDemoIndex()
	if err != nil || len(index.DemoRuns) == 0 {
		return "", false
	}
	demo := index.DemoRuns[0]
	if _, err := readJSONFile(filepath.Join(s.Config.DemoArtifactRoot, demo.BasePath, "report.json")); err != nil {
		return "", false
	}
	return "/api/demo-runs/" + demo.ID + "/report", true
}

func overallStatus(components []StatusComponent) StatusSummary {
	state := StatusStateOperational
	for _, component := range components {
		if statusRank(component.State) > statusRank(state) {
			state = component.State
		}
	}
	return StatusSummary{
		State:   state,
		Message: overallStatusMessage(state),
	}
}

func statusRank(state string) int {
	switch state {
	case StatusStateMajorOutage:
		return 5
	case StatusStatePartialOutage:
		return 4
	case StatusStateDegraded:
		return 3
	case StatusStateMaintenance:
		return 2
	case StatusStateUnknown:
		return 1
	default:
		return 0
	}
}

func overallStatusMessage(state string) string {
	switch state {
	case StatusStateMajorOutage:
		return "MealCheck is experiencing a major outage"
	case StatusStatePartialOutage:
		return "MealCheck is experiencing a partial outage"
	case StatusStateDegraded:
		return "MealCheck is experiencing degraded performance"
	case StatusStateMaintenance:
		return "MealCheck maintenance is in progress"
	case StatusStateUnknown:
		return "MealCheck status is partially unknown"
	default:
		return "All systems operational"
	}
}
