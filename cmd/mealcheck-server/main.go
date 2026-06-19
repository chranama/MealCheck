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

	"github.com/chranama/MealCheck/internal/hosted"
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
	config := hosted.ConfigFromEnv(root)
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

	store, err := hosted.OpenStore(ctx, config)
	if err != nil {
		return err
	}
	defer store.Close()

	pendingInputs := hosted.NewPendingInputs()
	providerFactory := hosted.DefaultProviderFactory
	if path := os.Getenv("MEALCHECK_FAKE_PROVIDER_RESPONSE_PATH"); path != "" {
		factory, err := hosted.StaticResponseProviderFactoryFromFile(path)
		if err != nil {
			return fmt.Errorf("load fake provider response: %w", err)
		}
		providerFactory = factory
	}
	worker := hosted.NewWorker(config, store, pendingInputs, providerFactory)
	cleanup := hosted.CleanupJob{Config: config, Store: store, Pending: pendingInputs}
	hostedServer := hosted.NewServer(config, store, pendingInputs)
	hostedServer.ProviderFactory = providerFactory
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
