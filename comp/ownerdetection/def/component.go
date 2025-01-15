package ownerdetection

import "context"

// Component is the component type.
type Component interface {
	Start(ctx context.Context) error
	Stop() error
}
