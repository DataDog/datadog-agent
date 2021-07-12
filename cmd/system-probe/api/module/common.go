package module

import (
	"errors"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

// ErrNotEnabled is a special error type that should be returned by a Factory
// when the associated Module is not enabled.
var ErrNotEnabled = errors.New("module is not enabled")

// Factory encapsulates the initialization of a Module
type Factory struct {
	Name config.ModuleName
	Fn   func(cfg *config.Config) (Module, error)
}

// Module defines the common API implemented by every System Probe Module
type Module interface {
	GetStats() map[string]interface{}
	Register(*Router) error
	Close()
}
