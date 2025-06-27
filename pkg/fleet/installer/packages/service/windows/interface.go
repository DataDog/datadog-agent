//go:build windows

// Package windows provides a set of functions to manage Windows services.
package windows

import (
	"context"
)

// ServiceManager interface abstracts all service management operations
type ServiceManager interface {
	StopAllAgentServices(ctx context.Context) error
	StartAgentServices(ctx context.Context) error
	RestartAgentServices(ctx context.Context) error
}
