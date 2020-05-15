package main

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Loader is responsible for managing the lifecyle of each api.Module, which includes:
// * Module initialization;
// * Module termination;
// * Module telemetry consolidation;
type Loader struct {
	modules map[string]api.Module
}

// Register a set of modules, which involves:
// * Initialization using the provided Factory;
// * Registering the HTTP endpoints of each module;
func (l *Loader) Register(cfg *config.AgentConfig, httpMux *http.ServeMux, factories []api.Factory) error {
	for _, factory := range factories {
		module, err := factory.Fn(cfg)

		// If the module is not enabled we simply skip to the next one
		if err == api.ErrNotEnabled {
			continue
		}

		if err != nil {
			return fmt.Errorf("new module `%s` error", factory.Name)
		}

		if err = module.Register(httpMux); err != nil {
			return fmt.Errorf("error registering gRPC endpoints for module `%s` error", factory.Name)
		}

		l.modules[factory.Name] = module

		log.Infof("module: %s started", factory.Name)
	}

	return nil
}

// GetStats returns the stats from all modules, namespaced by their names
func (l *Loader) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	for name, module := range l.modules {
		stats[name] = module.GetStats()
	}
	return stats
}

// Close each registered module
func (l *Loader) Close() {
	for _, module := range l.modules {
		module.Close()
	}
}

// NewLoader returns a new Loader instance
func NewLoader() *Loader {
	return &Loader{
		modules: make(map[string]api.Module),
	}
}
