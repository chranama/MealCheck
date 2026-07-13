package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/runs/submission"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	writeJSON(w, http.StatusOK, s.publicStatus(r.Context(), time.Now().UTC()))
}

func (s *Server) publicStatus(ctx context.Context, generatedAt time.Time) core.PublicStatusResponse {
	stats, storeErr := s.Store.Stats(ctx)
	queueAtCapacity := storeErr == nil && s.Config.QueueSize > 0 && stats.Queued >= s.Config.QueueSize
	localModelReady := s.publicLocalModelReady(ctx)
	sampleReportLink, sampleReportOK := s.sampleReportLink()

	components := []core.StatusComponent{
		mealCheckSubmissionStatus(storeErr, queueAtCapacity),
		aiMealNormalizationStatus(s.Config, storeErr, localModelReady),
		nutritionCheckingStatus(storeErr),
		reportGenerationStatus(s.Config, storeErr, queueAtCapacity, localModelReady),
		sampleReportStatus(sampleReportOK),
	}

	links := core.StatusLinks{}
	if sampleReportOK {
		links.SampleReport = sampleReportLink
	}

	return core.PublicStatusResponse{
		SchemaVersion:   "0.1",
		GeneratedAt:     generatedAt,
		Overall:         overallStatus(components),
		Components:      components,
		RecentIncidents: []core.StatusIncident{},
		Links:           links,
	}
}

func mealCheckSubmissionStatus(storeErr error, queueAtCapacity bool) core.StatusComponent {
	component := core.StatusComponent{ID: "meal_check_submission", Name: "Meal Check Submission"}
	switch {
	case storeErr != nil:
		component.State = core.StatusStateMajorOutage
		component.Message = "Meal checks cannot be accepted right now."
	case queueAtCapacity:
		component.State = core.StatusStateDegraded
		component.Message = "Meal checks are temporarily at capacity; retry shortly."
	default:
		component.State = core.StatusStateOperational
	}
	return component
}

func aiMealNormalizationStatus(config core.Config, storeErr error, localModelReady bool) core.StatusComponent {
	component := core.StatusComponent{ID: "ai_meal_normalization", Name: "AI Meal Normalization"}
	switch {
	case storeErr != nil:
		component.State = core.StatusStateMajorOutage
		component.Message = "AI meal normalization is unavailable while meal checks are offline."
	case submission.HostedMode(config) == core.HostedModeLocalModel && !localModelReady:
		component.State = core.StatusStatePartialOutage
		component.Message = "Server-owned AI meal normalization is currently unavailable."
	case submission.HostedMode(config) == core.HostedModeBYOK || submission.HostedMode(config) == core.HostedModeLocalModel:
		component.State = core.StatusStateOperational
	default:
		component.State = core.StatusStateUnknown
		component.Message = "AI meal normalization status is unknown."
	}
	return component
}

func nutritionCheckingStatus(storeErr error) core.StatusComponent {
	component := core.StatusComponent{ID: "nutrition_allergen_checking", Name: "Nutrition & Allergen Checking"}
	if storeErr != nil {
		component.State = core.StatusStateMajorOutage
		component.Message = "Nutrition and allergen checks cannot complete while meal checks are offline."
		return component
	}
	component.State = core.StatusStateOperational
	return component
}

func reportGenerationStatus(config core.Config, storeErr error, queueAtCapacity bool, localModelReady bool) core.StatusComponent {
	component := core.StatusComponent{ID: "report_generation", Name: "Report Generation"}
	switch {
	case storeErr != nil:
		component.State = core.StatusStateMajorOutage
		component.Message = "Reports cannot be generated right now."
	case queueAtCapacity:
		component.State = core.StatusStateDegraded
		component.Message = "Reports may be delayed because the meal-check queue is at capacity."
	case submission.HostedMode(config) == core.HostedModeLocalModel && !localModelReady:
		component.State = core.StatusStatePartialOutage
		component.Message = "Reports that need AI meal normalization may fail until the model is available."
	default:
		component.State = core.StatusStateOperational
	}
	return component
}

func sampleReportStatus(ok bool) core.StatusComponent {
	component := core.StatusComponent{ID: "sample_report", Name: "Sample Report"}
	if !ok {
		component.State = core.StatusStatePartialOutage
		component.Message = "The sample report is temporarily unavailable."
		return component
	}
	component.State = core.StatusStateOperational
	return component
}

func (s *Server) publicLocalModelReady(ctx context.Context) bool {
	if submission.HostedMode(s.Config) != core.HostedModeLocalModel {
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

func overallStatus(components []core.StatusComponent) core.StatusSummary {
	state := core.StatusStateOperational
	for _, component := range components {
		if statusRank(component.State) > statusRank(state) {
			state = component.State
		}
	}
	return core.StatusSummary{
		State:   state,
		Message: overallStatusMessage(state),
	}
}

func statusRank(state string) int {
	switch state {
	case core.StatusStateMajorOutage:
		return 5
	case core.StatusStatePartialOutage:
		return 4
	case core.StatusStateDegraded:
		return 3
	case core.StatusStateMaintenance:
		return 2
	case core.StatusStateUnknown:
		return 1
	default:
		return 0
	}
}

func overallStatusMessage(state string) string {
	switch state {
	case core.StatusStateMajorOutage:
		return "MealCheck is experiencing a major outage"
	case core.StatusStatePartialOutage:
		return "MealCheck is experiencing a partial outage"
	case core.StatusStateDegraded:
		return "MealCheck is experiencing degraded performance"
	case core.StatusStateMaintenance:
		return "MealCheck maintenance is in progress"
	case core.StatusStateUnknown:
		return "MealCheck status is partially unknown"
	default:
		return "All systems operational"
	}
}
