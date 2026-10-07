package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictAndAllowedPaths(t *testing.T) {
	root := t.TempDir()
	model := filepath.Join(root, "model.gguf")
	os.WriteFile(model, []byte("test"), 0600)
	digest := "lab/api@sha256:" + strings.Repeat("a", 64)
	d := Document{Version, "lab", "Running", Workload{"cpu-local-model-v1", digest, digest, digest, model, "lab", 18080, "Retain"}}
	p := Policy{ModelRoots: []string{root}, Registries: []string{"lab"}}
	if e := d.Validate(p); e != nil {
		t.Fatal(e)
	}
	if _, e := Decode([]byte(`{"unexpected":1}`)); e == nil {
		t.Fatal("unknown field")
	}
	if _, e := Decode([]byte(`{} {}`)); e == nil {
		t.Fatal("trailing JSON")
	}
	d.Spec.ModelPath = root
	if e := d.Validate(p); e == nil {
		t.Fatal("directory model accepted")
	}
	d.Spec.ModelPath = model
	d.Spec.APIImage = "lab/api:latest"
	if e := d.Validate(p); e == nil {
		t.Fatal("unpinned")
	}
	d.Spec.APIImage = digest
	outside := t.TempDir()
	target := filepath.Join(outside, "model")
	os.WriteFile(target, []byte("test"), 0600)
	link := filepath.Join(root, "link")
	os.Symlink(target, link)
	d.Spec.ModelPath = link
	if e := d.Validate(p); e == nil {
		t.Fatal("symlink escape")
	}
}
