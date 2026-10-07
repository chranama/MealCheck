package control

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/chranama/MealCheck/internal/infra/state"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSocketPermissionsAndProtocol(t *testing.T) {
	dir, err := os.MkdirTemp("/private/tmp", "mc-sock-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.Chmod(dir, 0700)
	s, e := state.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	path := filepath.Join(dir, "api.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, path, Handler(s, spec.Policy{})) }()
	deadline := time.Now().Add(time.Second)
	for {
		info, e := os.Stat(path)
		if e == nil {
			if info.Mode().Perm() != 0600 {
				t.Fatal(info.Mode())
			}
			break
		}
		if time.Now().After(deadline) {
			select {
			case err := <-done:
				t.Fatalf("socket missing: %v", err)
			default:
				t.Fatal("socket missing")
			}
		}
		time.Sleep(time.Millisecond)
	}
	c := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}}
	r, e := c.Post("http://unix/v1", "application/json", bytes.NewBufferString(`{"version":"other","command":"get"}`))
	if e != nil {
		t.Fatal(e)
	}
	var response Response
	if e = json.NewDecoder(r.Body).Decode(&response); e != nil || response.Error != "unsupported protocol version" {
		t.Fatal(response, e)
	}
	r.Body.Close()
	cancel()
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}
