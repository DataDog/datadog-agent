package delegatedauth

import (
	"context"
)

// New component to handle delegated token retrieval and propagation
type Component interface {
	GetApiKey(ctx context.Context) (*string, error)
	RefreshApiKey(ctx context.Context) error
}
