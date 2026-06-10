package hosted

import (
	"context"
	"fmt"
)

func OpenStore(ctx context.Context, config Config) (Store, error) {
	switch config.StoreKind {
	case "memory":
		return NewMemoryStore(), nil
	case "postgres", "":
		return OpenPostgresStore(ctx, config.DatabaseURL)
	default:
		return nil, fmt.Errorf("unsupported store kind %q", config.StoreKind)
	}
}
