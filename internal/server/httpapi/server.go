package httpapi

import (
	"net/http"

	"github.com/chranama/MealCheck/internal/access"
	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/inference"
	"github.com/chranama/MealCheck/internal/runs/runinput"
	"github.com/chranama/MealCheck/internal/state"
)

type Server struct {
	Config           core.Config
	Store            state.Store
	Inputs           *runinput.Vault
	Policy           *access.PolicyLimiter
	CompleterFactory inference.CompleterFactory
	mux              *http.ServeMux
}

func NewServer(config core.Config, store state.Store, inputs ...*runinput.Vault) *Server {
	inputVault := runinput.New()
	if len(inputs) > 0 && inputs[0] != nil {
		inputVault = inputs[0]
	}
	s := &Server{Config: config, Store: store, Inputs: inputVault, Policy: access.NewPolicyLimiter(), mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/demo-runs", s.handleDemoRuns)
	s.mux.HandleFunc("/api/demo-runs/", s.handleDemoRun)
	s.mux.HandleFunc("/api/qualify", s.handleQualify)
	s.mux.HandleFunc("/api/runs", s.handleRuns)
	s.mux.HandleFunc("/api/runs/", s.handleRun)
}
