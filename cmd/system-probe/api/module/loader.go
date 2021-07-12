package module

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

var l *loader

func init() {
	l = &loader{
		modules: make(map[config.ModuleName]Module),
	}
}

// loader is responsible for managing the lifecyle of each api.Module, which includes:
// * Module initialization;
// * Module termination;
// * Module telemetry consolidation;
type loader struct {
	sync.Mutex
	modules map[config.ModuleName]Module
	stats   map[string]interface{}
	cfg     *config.Config
	router  *Router
	closed  bool
}

// Register a set of modules, which involves:
// * Initialization using the provided Factory;
// * Registering the HTTP endpoints of each module;
func Register(cfg *config.Config, httpMux *mux.Router, factories []Factory) error {
	router := NewRouter(httpMux)
	for _, factory := range factories {
		if !cfg.ModuleIsEnabled(factory.Name) {
			log.Infof("%s module disabled", factory.Name)
			continue
		}

		module, err := factory.Fn(cfg)

		// In case a module failed to be started, do not make the whole `system-probe` abort.
		// Let `system-probe` run the other modules.
		if err != nil {
			log.Errorf("new module `%s` error: %s", factory.Name, err)
			continue
		}

		if err = module.Register(router); err != nil {
			log.Errorf("error registering HTTP endpoints for module `%s` error: %s", factory.Name, err)
			continue
		}

		l.modules[factory.Name] = module

		log.Infof("module: %s started", factory.Name)
	}

	l.router = router
	l.cfg = cfg
	if len(l.modules) == 0 {
		return errors.New("no module could be loaded")
	}

	go updateStats()
	return nil
}

// GetStats returns the stats from all modules, namespaced by their names
func GetStats() map[string]interface{} {
	l.Lock()
	defer l.Unlock()
	return l.stats
}

// RestartModule triggers a module restart
func RestartModule(factory Factory) error {
	l.Lock()
	defer l.Unlock()

	if l.closed == true {
		return fmt.Errorf("can't restart module because system-probe is shutting down")
	}

	currentModule := l.modules[factory.Name]
	if currentModule == nil {
		return fmt.Errorf("module %s is not running", factory.Name)
	}
	currentModule.Close()

	newModule, err := factory.Fn(l.cfg)
	if err != nil {
		return err
	}
	log.Infof("module %s restarted", factory.Name)

	err = newModule.Register(l.router)
	if err != nil {
		return err
	}

	l.modules[factory.Name] = newModule
	return nil
}

// Close each registered module
func Close() {
	l.Lock()
	defer l.Unlock()

	if l.closed == true {
		return
	}

	l.closed = true
	for _, module := range l.modules {
		module.Close()
	}
}

func updateStats() {
	then := time.Now()
	ticker := time.NewTicker(10 * time.Second)
	for now := range ticker.C {
		l.Lock()
		if l.closed {
			l.Unlock()
			return
		}

		l.stats = make(map[string]interface{})
		for name, module := range l.modules {
			l.stats[string(name)] = module.GetStats()
		}

		l.stats["updated_at"] = now
		l.stats["delta_seconds"] = now.Sub(then).Seconds()
		then = now
		l.Unlock()
	}
}
