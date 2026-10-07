package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestStaleSocketAndUnsafePaths(t *testing.T) {
	dir, e := os.MkdirTemp("/private/tmp", "mc-cli-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "controller.sock")
	os.WriteFile(path, []byte("preserve"), 0600)
	if e = prepareSocket(path); e == nil {
		t.Fatal("regular file accepted")
	}
	os.Remove(path)
	os.Symlink("missing", path)
	if e = prepareSocket(path); e == nil {
		t.Fatal("symlink accepted")
	}
	os.Remove(path)
	listener, e := net.Listen("unix", path)
	if e != nil {
		t.Fatal(e)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	if e = prepareSocket(path); e == nil {
		t.Fatal("live socket removed")
	}
	listener.Close()
	if e = prepareSocket(path); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("stale socket remains")
	}
}
