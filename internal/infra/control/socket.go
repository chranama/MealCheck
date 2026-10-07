package control

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/chranama/MealCheck/internal/infra/state"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

type Request struct {
	Version  string          `json:"version"`
	Command  string          `json:"command"`
	Document json.RawMessage `json:"document,omitempty"`
}
type Response struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func Handler(s *state.Store, p spec.Policy) http.Handler { return HandlerWithNotify(s, p, nil) }
func HandlerWithNotify(s *state.Store, p spec.Policy, notify func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond := func(v any, e error) {
			out := Response{Result: v}
			if e != nil {
				out = Response{Error: e.Error()}
			}
			json.NewEncoder(w).Encode(out)
		}
		if r.Method != "POST" || r.URL.Path != "/v1" {
			respond(nil, errors.New("unsupported request"))
			return
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, 65537))
		dec.DisallowUnknownFields()
		var q Request
		if e := dec.Decode(&q); e != nil {
			respond(nil, errors.New("invalid request"))
			return
		}
		if dec.Decode(new(any)) != io.EOF {
			respond(nil, errors.New("trailing request data"))
			return
		}
		if q.Version != spec.Version {
			respond(nil, errors.New("unsupported protocol version"))
			return
		}
		switch q.Command {
		case "apply":
			d, e := spec.Decode(q.Document)
			if e == nil {
				e = d.Validate(p)
			}
			if e != nil {
				respond(nil, e)
				return
			}
			canonical, err := filepath.EvalSymlinks(d.Spec.ModelPath)
			if err != nil {
				respond(nil, errors.New("model path cannot be resolved"))
				return
			}
			d.Spec.ModelPath = canonical
			v, e := s.Apply(d)
			if e == nil && notify != nil {
				notify()
			}
			respond(v, e)
		case "retry":
			v, e := s.Retry()
			if e == nil && notify != nil {
				notify()
			}
			respond(v, e)
		case "get":
			v, e := s.Get()
			respond(v, e)
		case "events":
			v, e := s.Events()
			respond(v, e)
		case "start", "stop", "delete":
			target := map[string]string{"start": "Running", "stop": "Stopped", "delete": "Deleted"}[q.Command]
			v, e := s.Transition(target)
			if e == nil && notify != nil {
				notify()
			}
			respond(v, e)
		default:
			respond(nil, errors.New("unknown command"))
		}
	})
}
func Serve(ctx context.Context, path string, h http.Handler) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	info, e := os.Stat(filepath.Dir(path))
	if e != nil {
		return e
	}
	if info.Mode().Perm()&0077 != 0 {
		return errors.New("socket parent must be private")
	}
	if _, e = os.Lstat(path); e == nil {
		return errors.New("socket already exists; remove only after confirming daemon is stopped")
	}
	old := sysUmask()
	ln, e := net.Listen("unix", path)
	restoreUmask(old)
	if e != nil {
		return e
	}
	defer ln.Close()
	defer os.Remove(path)
	if e = os.Chmod(path, 0600); e != nil {
		return e
	}
	server := &http.Server{Handler: h, ReadHeaderTimeout: 5e9, ReadTimeout: 5e9, WriteTimeout: 20e9}
	go func() { <-ctx.Done(); server.Close() }()
	e = server.Serve(ln)
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
