package core

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Root                      string
	DataDir                   string
	ArtifactDir               string
	Addr                      string
	DatabaseURL               string
	StoreKind                 string
	AllowedOrigin             string
	InviteToken               string
	InviteRequired            bool
	AccessMode                string
	HostedMode                string
	PublicOpenAICompatible    bool
	PublicRequestLimit        int
	PublicRequestWindow       time.Duration
	PublicDailyRunLimit       int
	MaxCandidateTextChars     int
	MaxGenerationPromptChars  int
	LocalModelEnabled         bool
	LocalModelBaseURL         string
	LocalModelName            string
	LocalModelTimeout         time.Duration
	LocalModelMaxInputChars   int
	LocalModelMaxSourceItems  int
	LocalModelMaxOutputTokens int
	QueueSize                 int
	MaxCasesPerRun            int
	MaxUploadBytes            int64
	RunTimeout                time.Duration
	PendingInputTTL           time.Duration
	Retention                 time.Duration
	WorkerPoll                time.Duration
	CleanupInterval           time.Duration
	DemoIndexPath             string
	DemoArtifactRoot          string
	FNDDSFallbackPath         string
}

func ConfigFromEnv(root string) Config {
	dataDir := getenv("MEALCHECK_DATA_DIR", filepath.Join(root, ".mealcheck-data"))
	artifactDir := getenv("MEALCHECK_ARTIFACT_DIR", filepath.Join(dataDir, "artifacts"))
	queueSize := getenvInt("MEALCHECK_QUEUE_SIZE", 3)
	runTimeout := getenvDuration("MEALCHECK_RUN_TIMEOUT", 10*time.Minute)
	inviteToken := os.Getenv("MEALCHECK_INVITE_TOKEN")
	inviteRequired := getenvBool("MEALCHECK_INVITE_REQUIRED", false)
	return Config{
		Root:                      root,
		DataDir:                   dataDir,
		ArtifactDir:               artifactDir,
		Addr:                      getenv("MEALCHECK_ADDR", "127.0.0.1:8080"),
		DatabaseURL:               os.Getenv("DATABASE_URL"),
		StoreKind:                 getenv("MEALCHECK_STORE", "postgres"),
		AllowedOrigin:             os.Getenv("MEALCHECK_ALLOWED_ORIGIN"),
		InviteToken:               inviteToken,
		InviteRequired:            inviteRequired,
		AccessMode:                accessModeFromEnv(inviteToken, inviteRequired),
		HostedMode:                hostedModeFromEnv(),
		PublicOpenAICompatible:    getenvBool("MEALCHECK_PUBLIC_OPENAI_COMPATIBLE", false),
		PublicRequestLimit:        getenvInt("MEALCHECK_PUBLIC_REQUEST_LIMIT", 60),
		PublicRequestWindow:       getenvDuration("MEALCHECK_PUBLIC_REQUEST_WINDOW", time.Minute),
		PublicDailyRunLimit:       getenvInt("MEALCHECK_PUBLIC_DAILY_RUN_LIMIT", 20),
		MaxCandidateTextChars:     getenvInt("MEALCHECK_MAX_CANDIDATE_TEXT_CHARS", 20_000),
		MaxGenerationPromptChars:  getenvInt("MEALCHECK_MAX_GENERATION_PROMPT_CHARS", 4_000),
		LocalModelEnabled:         getenvBool("MEALCHECK_LOCAL_MODEL_ENABLED", false),
		LocalModelBaseURL:         getenv("MEALCHECK_LOCAL_MODEL_BASE_URL", "http://127.0.0.1:11435/v1"),
		LocalModelName:            os.Getenv("MEALCHECK_LOCAL_MODEL_NAME"),
		LocalModelTimeout:         getenvDuration("MEALCHECK_LOCAL_MODEL_TIMEOUT", 240*time.Second),
		LocalModelMaxInputChars:   getenvInt("MEALCHECK_LOCAL_MODEL_MAX_INPUT_CHARS", 3_000),
		LocalModelMaxSourceItems:  getenvInt("MEALCHECK_LOCAL_MODEL_MAX_SOURCE_ITEMS", 20),
		LocalModelMaxOutputTokens: getenvInt("MEALCHECK_LOCAL_MODEL_MAX_OUTPUT_TOKENS", 1536),
		QueueSize:                 queueSize,
		MaxCasesPerRun:            getenvInt("MEALCHECK_MAX_CASES_PER_RUN", 20),
		MaxUploadBytes:            int64(getenvInt("MEALCHECK_MAX_UPLOAD_BYTES", 1_000_000)),
		RunTimeout:                runTimeout,
		PendingInputTTL:           getenvDuration("MEALCHECK_PENDING_INPUT_TTL", defaultPendingInputTTL(queueSize, runTimeout)),
		Retention:                 getenvDuration("MEALCHECK_RETENTION", 7*24*time.Hour),
		WorkerPoll:                getenvDuration("MEALCHECK_WORKER_POLL", time.Second),
		CleanupInterval:           getenvDuration("MEALCHECK_CLEANUP_INTERVAL", time.Hour),
		DemoIndexPath:             filepath.Join(root, "examples", "seeded-3-day-peanut-allergy", "artifacts", "demo-runs", "index.json"),
		DemoArtifactRoot:          filepath.Join(root, "examples", "seeded-3-day-peanut-allergy", "artifacts"),
		FNDDSFallbackPath:         os.Getenv("MEALCHECK_FNDDS_FALLBACK_PATH"),
	}
}

func hostedModeFromEnv() string {
	switch value := os.Getenv("MEALCHECK_HOSTED_MODE"); value {
	case HostedModeBYOK, HostedModeLocalModel:
		return value
	case "":
		return HostedModeBYOK
	default:
		return HostedModeBYOK
	}
}

func accessModeFromEnv(inviteToken string, inviteRequired bool) string {
	switch value := os.Getenv("MEALCHECK_ACCESS_MODE"); value {
	case AccessModePublicBYOK, AccessModeInviteRequired:
		return value
	case "":
		if inviteToken != "" || inviteRequired {
			return AccessModeInviteRequired
		}
		return AccessModePublicBYOK
	default:
		return AccessModeInviteRequired
	}
}

func defaultPendingInputTTL(queueSize int, runTimeout time.Duration) time.Duration {
	if queueSize < 1 {
		queueSize = 1
	}
	if runTimeout <= 0 {
		runTimeout = 10 * time.Minute
	}
	ttl := time.Duration(queueSize+1) * runTimeout
	if ttl < 15*time.Minute {
		return 15 * time.Minute
	}
	return ttl
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
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
