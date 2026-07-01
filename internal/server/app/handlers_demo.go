package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleDemoRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	index, err := s.loadDemoIndex()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "demo_index_unavailable", err.Error(), nil)
		return
	}
	type demoResponse struct {
		DemoRun
		Links RunLinks `json:"links"`
	}
	response := struct {
		SchemaVersion string         `json:"schema_version"`
		DemoRuns      []demoResponse `json:"demo_runs"`
	}{SchemaVersion: index.SchemaVersion}
	for _, demo := range index.DemoRuns {
		response.DemoRuns = append(response.DemoRuns, demoResponse{
			DemoRun: demo,
			Links: RunLinks{
				Self:      "/api/demo-runs/" + demo.ID,
				Report:    "/api/demo-runs/" + demo.ID + "/report",
				Artifacts: "/api/demo-runs/" + demo.ID + "/artifacts",
			},
		})
	}
	writeJSON(w, http.StatusOK, response)
}
func (s *Server) handleDemoRun(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/demo-runs/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, r, http.StatusNotFound, "not_found", "demo run not found", nil)
		return
	}
	demo, ok, err := s.findDemo(parts[0])
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "demo_index_unavailable", err.Error(), nil)
		return
	}
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "demo run not found", nil)
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		decision, err := readJSONFile(filepath.Join(s.Config.DemoArtifactRoot, demo.BasePath, "decision.json"))
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "demo_unavailable", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"demo": demo, "decision": decision})
		return
	}
	switch parts[1] {
	case "report":
		s.serveDemoReport(w, r, demo)
	case "artifacts":
		s.serveDemoArtifact(w, r, demo, strings.Join(parts[2:], "/"))
	default:
		writeError(w, r, http.StatusNotFound, "not_found", "demo route not found", nil)
	}
}
func (s *Server) serveDemoReport(w http.ResponseWriter, r *http.Request, demo DemoRun) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	serveArtifactFile(w, r, s.Config.DemoArtifactRoot, filepath.Join(s.Config.DemoArtifactRoot, demo.BasePath, "report.json"))
}
func (s *Server) serveDemoArtifact(w http.ResponseWriter, r *http.Request, demo DemoRun, artifactPath string) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	base := filepath.Join(s.Config.DemoArtifactRoot, demo.BasePath)
	if artifactPath == "" {
		s.listArtifactManifest(w, r, demo.ID, base, "/api/demo-runs/"+demo.ID+"/artifacts")
		return
	}
	path, err := safeArtifactPath(base, artifactPath)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_artifact_path", err.Error(), nil)
		return
	}
	serveArtifactFile(w, r, s.Config.DemoArtifactRoot, path)
}
func (s *Server) loadDemoIndex() (DemoIndex, error) {
	var index DemoIndex
	b, err := os.ReadFile(s.Config.DemoIndexPath)
	if err != nil {
		return DemoIndex{}, err
	}
	if err := json.Unmarshal(b, &index); err != nil {
		return DemoIndex{}, err
	}
	return index, nil
}
func (s *Server) findDemo(id string) (DemoRun, bool, error) {
	index, err := s.loadDemoIndex()
	if err != nil {
		return DemoRun{}, false, err
	}
	for _, demo := range index.DemoRuns {
		if demo.ID == id {
			return demo, true, nil
		}
	}
	return DemoRun{}, false, nil
}
