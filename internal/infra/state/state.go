// Package state keeps control-plane state independent of the workload database.
package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Record struct {
	Document   spec.Document `json:"document"`
	Generation int64         `json:"generation"`
	Tombstone  bool          `json:"tombstone"`
	Status     Status        `json:"status"`
}
type Condition struct {
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason"`
	Generation     int64     `json:"generation"`
	TransitionTime time.Time `json:"transitionTime"`
}
type Binding struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Retained bool   `json:"retained"`
}
type Status struct {
	RetryEpoch         int64                  `json:"retryEpoch"`
	Phase              string                 `json:"phase"`
	ObservedGeneration int64                  `json:"observedGeneration"`
	Conditions         []Condition            `json:"conditions"`
	Resources          map[string]Binding     `json:"resources"`
	LastObservation    time.Time              `json:"lastObservation"`
	RetryCount         int                    `json:"retryCount"`
	NextRetry          time.Time              `json:"nextRetry"`
	Starts             map[string][]time.Time `json:"starts,omitempty"`
	StartupSince       map[string]time.Time   `json:"startupSince,omitempty"`
	Failure            string                 `json:"failure,omitempty"`
}
type Operation struct {
	ID          string `json:"id"`
	Generation  int64  `json:"generation"`
	Role        string `json:"role"`
	Action      string `json:"action"`
	Fingerprint string `json:"fingerprint"`
	Outcome     string `json:"outcome"`
}
type Event struct {
	ID         int64  `json:"id"`
	Time       string `json:"time"`
	Reason     string `json:"reason"`
	Generation int64  `json:"generation"`
}
type Store struct {
	DB             *sql.DB
	lock           *os.File
	mu             sync.Mutex
	InstallationID string
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "controller.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("controller state already locked")
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "controller.sqlite"))
	if err != nil {
		f.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, lock: f}
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY,value TEXT NOT NULL); CREATE TABLE IF NOT EXISTS deployment (id INTEGER PRIMARY KEY CHECK(id=1),body TEXT NOT NULL); CREATE TABLE IF NOT EXISTS operations (id TEXT PRIMARY KEY,body TEXT NOT NULL); CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT,time TEXT NOT NULL,reason TEXT NOT NULL,generation INTEGER NOT NULL);`)
	if err != nil {
		s.Close()
		return nil, err
	}
	if err := os.Chmod(filepath.Join(dir, "controller.sqlite"), 0600); err != nil {
		s.Close()
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		s.Close()
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT OR IGNORE INTO metadata VALUES ('schema','1'),('installation',?)`, uuid.NewString())
	if err == nil {
		var version string
		err = tx.QueryRow(`SELECT value FROM metadata WHERE key='schema'`).Scan(&version)
		if version != "1" {
			err = errors.New("unsupported state schema")
		}
	}
	if err == nil {
		err = tx.QueryRow(`SELECT value FROM metadata WHERE key='installation'`).Scan(&s.InstallationID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error {
	err := s.DB.Close()
	unix.Flock(int(s.lock.Fd()), unix.LOCK_UN)
	s.lock.Close()
	return err
}
func (s *Store) Get() (Record, error) {
	var r Record
	var b string
	err := s.DB.QueryRow(`SELECT body FROM deployment WHERE id=1`).Scan(&b)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal([]byte(b), &r)
	return r, err
}
func write(tx *sql.Tx, r Record) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = tx.Exec(`INSERT INTO deployment VALUES(1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body`, string(b))
	return e
}
func event(tx *sql.Tx, reason string, g int64) error {
	_, e := tx.Exec(`INSERT INTO events(time,reason,generation) VALUES(?,?,?)`, time.Now().UTC().Format(time.RFC3339Nano), reason, g)
	if e == nil {
		_, e = tx.Exec(`DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT 1000)`)
	}
	return e
}
func (s *Store) Apply(d spec.Document) (Record, error) {
	if d.DesiredState != "Running" && d.DesiredState != "Stopped" && d.DesiredState != "Deleted" {
		return Record{}, errors.New("unsupported desiredState")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, e := s.Get()
	if e != nil && e != sql.ErrNoRows {
		return old, e
	}
	if e == nil {
		if old.Document.DeploymentID != d.DeploymentID {
			return old, errors.New("one deployment per controller")
		}
		if old.Tombstone {
			return old, errors.New("deployment is terminally deleted")
		}
		if old.Document.Spec != d.Spec {
			return old, errors.New("workload configuration is immutable")
		}
		if old.Document.Spec == d.Spec && old.Document.APIVersion == d.APIVersion && old.Document.DeploymentID == d.DeploymentID && old.Document.DesiredState == d.DesiredState {
			return old, nil
		}
	}
	r := old
	r.Document = d
	r.Generation++
	r.Tombstone = d.DesiredState == "Deleted"
	r.Status.Phase = "Pending"
	r.Status.NextRetry = time.Time{}
	r.Status.RetryCount = 0
	r.Status.Failure = ""
	for i := range r.Status.Conditions {
		r.Status.Conditions[i].Status = "False"
		r.Status.Conditions[i].Reason = "DesiredStateChanged"
		r.Status.Conditions[i].Generation = r.Generation
		r.Status.Conditions[i].TransitionTime = time.Now().UTC()
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return old, e
	}
	defer tx.Rollback()
	if e = write(tx, r); e == nil {
		e = event(tx, "DesiredStateAccepted", r.Generation)
	}
	if e == nil {
		e = tx.Commit()
	}
	return r, e
}
func (s *Store) Transition(desired string) (Record, error) {
	r, e := s.Get()
	if e != nil {
		return r, e
	}
	if r.Tombstone && desired == "Deleted" {
		return r, nil
	}
	r.Document.DesiredState = desired
	return s.Apply(r.Document)
}
func (s *Store) SetStatus(g int64, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.Get()
	if e != nil {
		return e
	}
	if r.Status.RetryEpoch != status.RetryEpoch {
		return nil
	}
	if r.Generation != g {
		status.Phase = "Pending"
		status.ObservedGeneration = g
	}
	for i := range status.Conditions {
		for _, old := range r.Status.Conditions {
			if old.Type == status.Conditions[i].Type && old.Status == status.Conditions[i].Status && old.Reason == status.Conditions[i].Reason && old.Generation == status.Conditions[i].Generation {
				status.Conditions[i].TransitionTime = old.TransitionTime
				break
			}
		}
	}
	r.Status = status
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = write(tx, r); e == nil {
		e = tx.Commit()
	}
	return e
}
func (s *Store) Journal(o Operation) error {
	b, e := json.Marshal(o)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(`INSERT INTO operations VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body`, o.ID, string(b))
	if e == nil {
		_, e = s.DB.Exec(`DELETE FROM operations WHERE id NOT IN (SELECT id FROM operations ORDER BY rowid DESC LIMIT 1000) AND json_extract(body,'$.outcome') != ''`)
	}
	return e
}
func (s *Store) Operations() ([]Operation, error) {
	rows, e := s.DB.Query(`SELECT body FROM operations`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Operation
	for rows.Next() {
		var b string
		var o Operation
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &o); e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (s *Store) AddEvent(reason string, g int64) error {
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = event(tx, reason, g); e == nil {
		e = tx.Commit()
	}
	return e
}
func (s *Store) Events() ([]Event, error) {
	rows, e := s.DB.Query(`SELECT id,time,reason,generation FROM events ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var v Event
		if e = rows.Scan(&v.ID, &v.Time, &v.Reason, &v.Generation); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Retry explicitly resets operator-intervention budgets, without changing generation.
func (s *Store) Retry() (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.Get()
	if e != nil {
		return r, e
	}
	if r.Tombstone {
		return r, errors.New("deleted deployment cannot retry")
	}
	r.Status.RetryEpoch++
	r.Status.RetryCount = 0
	r.Status.NextRetry = time.Time{}
	r.Status.Starts = nil
	r.Status.StartupSince = nil
	r.Status.Failure = ""
	r.Status.Phase = "Pending"
	for i := range r.Status.Conditions {
		r.Status.Conditions[i].Status = "False"
		r.Status.Conditions[i].Reason = "OperatorRetry"
		r.Status.Conditions[i].TransitionTime = time.Now().UTC()
	}
	tx, e := s.DB.Begin()
	if e != nil {
		return r, e
	}
	defer tx.Rollback()
	if e = write(tx, r); e == nil {
		e = event(tx, "OperatorRetry", r.Generation)
	}
	if e == nil {
		e = tx.Commit()
	}
	return r, e
}
