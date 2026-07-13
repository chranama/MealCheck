package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/chranama/MealCheck/internal/core"
	"github.com/chranama/MealCheck/internal/llm/client"
	"github.com/chranama/MealCheck/internal/runs/execution"
	"github.com/chranama/MealCheck/internal/runs/runinput"
	"github.com/chranama/MealCheck/internal/server/httpapi"
	"github.com/chranama/MealCheck/internal/state"
	"github.com/chranama/MealCheck/internal/state/memory"
	"github.com/chranama/MealCheck/internal/state/postgres"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "mealcheck-server failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("mealcheck-server", flag.ContinueOnError)
	rootFlag := flags.String("root", ".", "repository root")
	addrFlag := flags.String("addr", "", "HTTP bind address")
	dataDirFlag := flags.String("data-dir", "", "runtime data directory")
	artifactDirFlag := flags.String("artifact-dir", "", "artifact storage directory")
	storeFlag := flags.String("store", "", "metadata store: postgres or memory")
	if err := flags.Parse(args); err != nil {
		return err
	}

	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		return err
	}
	config := core.ConfigFromEnv(root)
	if *addrFlag != "" {
		config.Addr = *addrFlag
	}
	if *dataDirFlag != "" {
		config.DataDir = *dataDirFlag
	}
	if *artifactDirFlag != "" {
		config.ArtifactDir = *artifactDirFlag
	}
	if *storeFlag != "" {
		config.StoreKind = *storeFlag
	}
	if err := os.MkdirAll(config.DataDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(config.ArtifactDir, 0o755); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stateStore, err := openStateStore(ctx, config)
	if err != nil {
		return err
	}
	defer stateStore.Close()

	inputVault := runinput.New()
	completerFactory := client.New
	if path := os.Getenv("MEALCHECK_FAKE_PROVIDER_RESPONSE_PATH"); path != "" {
		factory, err := client.StaticResponseFactoryFromFile(path)
		if err != nil {
			return fmt.Errorf("load fake provider response: %w", err)
		}
		completerFactory = factory
	}
	worker := execution.NewWorker(config, stateStore, inputVault, completerFactory)
	cleanup := execution.CleanupJob{Config: config, Store: stateStore, Inputs: inputVault}
	hostedServer := httpapi.NewServer(config, stateStore, inputVault)
	hostedServer.CompleterFactory = completerFactory
	go worker.Run(ctx)
	go cleanup.Run(ctx)

	server := &http.Server{
		Addr:              config.Addr,
		Handler:           hostedServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stdout, "mealcheck-server listening on http://%s\n", config.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func openStateStore(ctx context.Context, config core.Config) (state.Store, error) {
	switch config.StoreKind {
	case "memory":
		return memory.New(), nil
	case "postgres", "":
		return postgres.Open(ctx, config.DatabaseURL)
	default:
		return nil, fmt.Errorf("unsupported store kind %q", config.StoreKind)
	}
}
