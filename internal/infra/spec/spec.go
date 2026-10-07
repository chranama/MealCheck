// Package spec defines the deliberately narrow, versioned workload contract.
package spec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const Version = "mealcheck.dev/v1alpha1"

type Document struct {
	APIVersion   string   `json:"apiVersion"`
	DeploymentID string   `json:"deploymentID"`
	DesiredState string   `json:"desiredState"`
	Spec         Workload `json:"spec"`
}
type Workload struct {
	Profile       string `json:"profile"`
	APIImage      string `json:"apiImage"`
	PostgresImage string `json:"postgresImage"`
	ModelImage    string `json:"modelImage"`
	ModelPath     string `json:"modelPath"`
	SecretProfile string `json:"secretProfile"`
	APIHostPort   int    `json:"apiHostPort"`
	DataPolicy    string `json:"dataPolicy"`
}
type Policy struct {
	ModelRoots       []string
	Registries       []string
	MinPort, MaxPort int
}

var id = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var image = regexp.MustCompile(`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`)

func Decode(b []byte) (Document, error) {
	var d Document
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return d, errors.New("expected one JSON document")
	}
	return d, nil
}
func (d Document) Validate(p Policy) error {
	if d.APIVersion != Version {
		return errors.New("unsupported apiVersion")
	}
	if !id.MatchString(d.DeploymentID) || !id.MatchString(d.Spec.SecretProfile) {
		return errors.New("invalid deployment or secret profile ID")
	}
	if d.DesiredState != "Running" && d.DesiredState != "Stopped" && d.DesiredState != "Deleted" {
		return errors.New("unsupported desiredState")
	}
	if d.Spec.Profile != "cpu-local-model-v1" || d.Spec.DataPolicy != "Retain" {
		return errors.New("unsupported workload profile or data policy")
	}
	for _, v := range []string{d.Spec.APIImage, d.Spec.PostgresImage, d.Spec.ModelImage} {
		if !image.MatchString(v) {
			return errors.New("images require sha256 digest")
		}
		allowed := false
		for _, r := range p.Registries {
			if strings.HasPrefix(v, r+"/") || strings.HasPrefix(v, r+"@") {
				allowed = true
			}
		}
		if !allowed {
			return errors.New("image registry is not allowed")
		}
	}
	if p.MinPort == 0 {
		p.MinPort = 1024
	}
	if p.MaxPort == 0 {
		p.MaxPort = 65535
	}
	if d.Spec.APIHostPort < p.MinPort || d.Spec.APIHostPort > p.MaxPort {
		return errors.New("port outside allowed range")
	}
	if !filepath.IsAbs(d.Spec.ModelPath) {
		return errors.New("model path must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(d.Spec.ModelPath)
	if err != nil {
		return errors.New("model file cannot be resolved")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("model path must reference a regular file")
	}
	allowed := false
	for _, root := range p.ModelRoots {
		rr, e := filepath.EvalSymlinks(root)
		if e != nil {
			continue
		}
		rel, e := filepath.Rel(rr, resolved)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("model path outside allowed roots")
	}
	return nil
}
func (w Workload) Fingerprint() string {
	b, _ := json.Marshal(w)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
