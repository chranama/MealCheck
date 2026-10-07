package state

import (
	"encoding/json"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"testing"
	"time"
)

func TestOpenPreM7StateKeepsGoldenOwnershipAndRecovery(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// This wire fixture deliberately contains none of the new M7 fields. Insert
	// old bytes directly, rather than passing through today's struct serializer.
	body := `{"document":{"apiVersion":"mealcheck.dev/v1alpha1","deploymentID":"lab","desiredState":"Running","spec":{"profile":"cpu-local-model-v1","apiImage":"lab/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","postgresImage":"lab/pg@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","modelImage":"lab/model@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","modelPath":"/legacy/model.gguf","secretProfile":"lab","apiHostPort":18080,"dataPolicy":"Retain"}},"generation":9,"tombstone":false,"status":{"phase":"Blocked","retryEpoch":4,"retryCount":3,"failure":"modelStartBudgetExhausted","resources":{"database-volume":{"id":"original-volume","name":"original-name","retained":false}},"starts":{"model":["2026-10-07T00:00:00Z"]}}}`
	if _, err = s.DB.Exec(`INSERT INTO deployment(id,body) VALUES(1,?)`, body); err != nil {
		t.Fatal(err)
	}
	if err = s.Journal(Operation{ID: "unfinished", Generation: 9, Role: "model", Action: "start", Fingerprint: "d5e53b1d471237a0122d1aef83035aef7942634c0598f36a43fc28e6ab6d9f2f"}); err != nil {
		t.Fatal(err)
	}
	installation := s.InstallationID
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got.Document.Spec.Fingerprint() != "d5e53b1d471237a0122d1aef83035aef7942634c0598f36a43fc28e6ab6d9f2f" || s.InstallationID != installation {
		t.Fatal("legacy ownership changed")
	}
	var before Record
	if err = json.Unmarshal([]byte(body), &before); err != nil {
		t.Fatal(err)
	}
	got, err = s.Apply(got.Document)
	if err != nil || got.Generation != 9 || got.Status.RetryEpoch != 4 || got.Status.RetryCount != 3 || got.Status.Failure != before.Status.Failure || got.Status.Resources["database-volume"].ID != "original-volume" || len(got.Status.Starts["model"]) != 1 {
		t.Fatal("legacy recovery reset", got, err)
	}
	ops, err := s.Operations()
	if err != nil || len(ops) != 1 || ops[0].ID != "unfinished" {
		t.Fatal("legacy intent lost", ops, err)
	}
}

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

func TestM7LegacyAndSnapshotReopenPreserveRecovery(t *testing.T) {
	for _, snapshot := range []string{"", `{"system":{"name":"lab","version":"v1","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"architecture":"arm64","applicationRole":"api","resources":[]}`} {
		t.Run(snapshot, func(t *testing.T) {
			dir := t.TempDir()
			s, e := Open(dir)
			if e != nil {
				t.Fatal(e)
			}
			d := spec.Document{APIVersion: spec.Version, DeploymentID: "lab", DesiredState: "Running", Spec: spec.Workload{Profile: "cpu-local-model-v1", ResolvedJSON: snapshot}, DeploymentJSON: `{"accepted":"original"}`}
			r, e := s.Apply(d)
			if e != nil {
				t.Fatal(e)
			}
			r.Status.RetryCount = 3
			r.Status.RetryEpoch = 7
			r.Status.Failure = "startup exhausted"
			r.Status.Resources = map[string]Binding{"database-volume": {ID: "original-incarnation", Name: "retained", Retained: true}}
			r.Status.Starts = map[string][]time.Time{"model": {time.Unix(10, 0).UTC()}}
			tx, err := s.DB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if e = write(tx, r); e != nil {
				t.Fatal(e)
			}
			if e = tx.Commit(); e != nil {
				t.Fatal(e)
			}
			if e = s.Journal(Operation{ID: "pending", Generation: r.Generation, Role: "model", Action: "start", Fingerprint: d.Spec.Fingerprint()}); e != nil {
				t.Fatal(e)
			}
			installation, fingerprint := s.InstallationID, d.Spec.Fingerprint()
			s.Close()
			s, e = Open(dir)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			got, e := s.Get()
			if e != nil {
				t.Fatal(e)
			}
			if s.InstallationID != installation || got.Document.Spec.Fingerprint() != fingerprint || got.Document.Spec.ResolvedJSON != snapshot || got.Document.DeploymentJSON != d.DeploymentJSON {
				t.Fatal("durable configuration or identity changed")
			}
			d.DeploymentJSON = `{"accepted":"equivalent"}`
			got, e = s.Apply(d)
			if e != nil || got.Generation != 1 || got.Status.RetryCount != 3 || got.Status.RetryEpoch != 7 || got.Status.Resources["database-volume"].ID != "original-incarnation" || len(got.Status.Starts["model"]) != 1 {
				t.Fatal("identical apply reset durable recovery", got, e)
			}
			ops, e := s.Operations()
			if e != nil || len(ops) != 1 || ops[0].ID != "pending" {
				t.Fatal("journal lost", e)
			}
			changed := d
			changed.Spec.ResolvedJSON = "changed"
			if _, e = s.Apply(changed); e == nil {
				t.Fatal("accepted immutable snapshot change")
			}
			if _, e = s.Transition("Deleted"); e != nil {
				t.Fatal(e)
			}
			if _, e = s.Apply(d); e == nil {
				t.Fatal("resurrected tombstone")
			}
		})
	}
}
