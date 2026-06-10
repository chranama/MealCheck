package hosted

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func ConfigFromEnv(root string) Config {
	dataDir := getenv("MEALCHECK_DATA_DIR", filepath.Join(root, ".mealcheck-data"))
	artifactDir := getenv("MEALCHECK_ARTIFACT_DIR", filepath.Join(dataDir, "artifacts"))
	return Config{
		Root:             root,
		DataDir:          dataDir,
		ArtifactDir:      artifactDir,
		Addr:             getenv("MEALCHECK_ADDR", "127.0.0.1:8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		StoreKind:        getenv("MEALCHECK_STORE", "postgres"),
		AllowedOrigin:    os.Getenv("MEALCHECK_ALLOWED_ORIGIN"),
		InviteToken:      os.Getenv("MEALCHECK_INVITE_TOKEN"),
		QueueSize:        getenvInt("MEALCHECK_QUEUE_SIZE", 3),
		MaxCasesPerRun:   getenvInt("MEALCHECK_MAX_CASES_PER_RUN", 20),
		MaxUploadBytes:   int64(getenvInt("MEALCHECK_MAX_UPLOAD_BYTES", 1_000_000)),
		RunTimeout:       getenvDuration("MEALCHECK_RUN_TIMEOUT", 10*time.Minute),
		Retention:        getenvDuration("MEALCHECK_RETENTION", 7*24*time.Hour),
		WorkerPoll:       getenvDuration("MEALCHECK_WORKER_POLL", time.Second),
		CleanupInterval:  getenvDuration("MEALCHECK_CLEANUP_INTERVAL", time.Hour),
		DemoIndexPath:    filepath.Join(root, "ui", "demo-runs", "index.json"),
		DemoArtifactRoot: filepath.Join(root, "ui"),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
