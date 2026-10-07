package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/chranama/MealCheck/internal/infra/control"
	"github.com/chranama/MealCheck/internal/infra/controller"
	dockerprovider "github.com/chranama/MealCheck/internal/infra/provider/docker"
	"github.com/chranama/MealCheck/internal/infra/spec"
	"github.com/chranama/MealCheck/internal/infra/state"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: mealcheck-controller daemon|apply|get|start|stop|delete|events")
	}
	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	dir := fs.String("state-dir", filepath.Join(os.Getenv("HOME"), ".mealcheck-controller"), "private state directory")
	socket := fs.String("socket", "", "Unix socket path")
	file := fs.String("file", "", "desired-state JSON file")
	roots := fs.String("model-roots", "", "comma-separated allowed model directories")
	engine := fs.String("engine", "", "explicit local Docker Engine Unix endpoint")
	secretRoot := fs.String("secret-root", "", "root of private secret profiles")
	registries := fs.String("registries", "", "comma-separated approved image prefixes")
	if e := fs.Parse(args[1:]); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *socket == "" {
		*socket = filepath.Join(*dir, "controller.sock")
	}
	if command == "daemon" {
		if *roots == "" || *registries == "" || *engine == "" || *secretRoot == "" {
			return errors.New("daemon requires --model-roots, --registries, --engine, and --secret-root")
		}
		s, e := state.Open(*dir)
		if e != nil {
			return e
		}
		defer s.Close()
		if err := prepareSocket(*socket); err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		p, err := dockerprovider.New(*engine, *secretRoot)
		if err != nil {
			return err
		}
		defer p.Close()
		wake := make(chan struct{}, 1)
		reconcile := &controller.Controller{Store: s, Provider: p, Wake: wake}
		done := make(chan error, 1)
		go func() { err := reconcile.Run(ctx, 5*time.Second); done <- err; cancel() }()
		serveErr := control.Serve(ctx, *socket, control.HandlerWithNotify(s, spec.Policy{ModelRoots: strings.Split(*roots, ","), Registries: strings.Split(*registries, ",")}, func() {
			select {
			case wake <- struct{}{}:
			default:
			}
		}))
		cancel()
		reconcileErr := <-done
		if serveErr != nil {
			return serveErr
		}
		return reconcileErr
	}
	q := control.Request{Version: spec.Version, Command: command}
	if command == "apply" {
		if *file == "" {
			return errors.New("apply requires --file")
		}
		b, e := os.ReadFile(*file)
		if e != nil {
			return e
		}
		q.Document = b
	}
	b, e := json.Marshal(q)
	if e != nil {
		return e
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", *socket)
	}}}
	resp, e := client.Post("http://unix/v1", "application/json", bytes.NewReader(b))
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if e != nil {
		return e
	}
	var result control.Response
	if e = json.Unmarshal(data, &result); e != nil {
		return e
	}
	if result.Error != "" {
		return errors.New(result.Error)
	}
	out, e := json.MarshalIndent(result.Result, "", "  ")
	if e == nil {
		fmt.Println(string(out))
	}
	return e
}

// prepareSocket is called only after obtaining the lifetime state lock.
func prepareSocket(path string) error {
	parentInfo, err := os.Stat(filepath.Dir(path))
	if err != nil || parentInfo.Mode().Perm()&0077 != 0 {
		return errors.New("socket parent must exist and be private")
	}
	if stat, ok := parentInfo.Sys().(*syscall.Stat_t); !ok || stat.Uid != uint32(os.Getuid()) {
		return errors.New("socket parent not owned by current user")
	}
	// The state lock establishes that no daemon for this directory is alive.
	if info, err := os.Lstat(path); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Uid != uint32(os.Getuid()) {
			return errors.New("socket not owned by current user")
		}
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("socket path is not a socket")
		}
		conn, err := net.DialTimeout("unix", path, time.Second)
		if err == nil {
			conn.Close()
			return errors.New("socket already has a listener")
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}

	return nil
}
