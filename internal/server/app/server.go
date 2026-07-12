package app

import "net/http"

type Server struct {
	Config           Config
	Store            Store
	Pending          *PendingInputs
	Policy           *PolicyLimiter
	CompleterFactory CompleterFactory
	mux              *http.ServeMux
}

func NewServer(config Config, store Store, pending ...*PendingInputs) *Server {
	pendingInputs := NewPendingInputs()
	if len(pending) > 0 && pending[0] != nil {
		pendingInputs = pending[0]
	}
	s := &Server{Config: config, Store: store, Pending: pendingInputs, Policy: NewPolicyLimiter(), mux: http.NewServeMux()}
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
