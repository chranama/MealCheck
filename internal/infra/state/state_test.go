package state

import (
	"github.com/chranama/MealCheck/internal/infra/spec"
	"testing"
)

func TestDurableTransitionsAndLock(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Open(dir); e == nil {
		t.Fatal("second writer acquired lock")
	}
	d := spec.Document{APIVersion: spec.Version, DeploymentID: "lab", DesiredState: "Running", Spec: spec.Workload{Profile: "cpu-local-model-v1"}}
	r, e := s.Apply(d)
	if e != nil || r.Generation != 1 {
		t.Fatal(r, e)
	}
	r, e = s.Apply(d)
	if e != nil || r.Generation != 1 {
		t.Fatal("identical apply", r, e)
	}
	changed := d
	changed.Spec.APIHostPort = 1234
	if _, e = s.Apply(changed); e == nil {
		t.Fatal("immutable update accepted")
	}
	installation := s.InstallationID
	s.Close()
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.InstallationID != installation {
		t.Fatal("identity changed")
	}
	r, e = s.Get()
	if e != nil || r.Generation != 1 {
		t.Fatal(r, e)
	}
	r, e = s.Transition("Deleted")
	if e != nil || !r.Tombstone || r.Generation != 2 {
		t.Fatal(r, e)
	}
	if _, e = s.Transition("Running"); e == nil {
		t.Fatal("resurrected")
	}
	r, e = s.Transition("Deleted")
	if e != nil || r.Generation != 2 {
		t.Fatal(r, e)
	}
}
func TestEventsBounded(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := 0; i < 1005; i++ {
		if e = s.AddEvent("test", 1); e != nil {
			t.Fatal(e)
		}
	}
	events, e := s.Events()
	if e != nil || len(events) != 1000 {
		t.Fatal(len(events), e)
	}
}
