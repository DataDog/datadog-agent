// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var l *loader

func init() {
	l = &loader{
		modules: make(map[config.ModuleName]Module),
		errors:  make(map[config.ModuleName]error),
		routers: make(map[config.ModuleName]*Router),
	}
}

// loader is responsible for managing the lifecycle of each api.Module, which includes:
// * Module initialization;
// * Module termination;
// * Module telemetry consolidation;
type loader struct {
	sync.Mutex
	modules map[config.ModuleName]Module
	errors  map[config.ModuleName]error
	stats   map[string]interface{}
	cfg     *config.Config
	routers map[config.ModuleName]*Router
	closed  bool
}

// Register a set of modules, which involves:
// * Initialization using the provided Factory;
// * Registering the HTTP endpoints of each module;
func Register(cfg *config.Config, httpMux *mux.Router, factories []Factory) error {
	driver.Init(cfg)

	for _, factory := range factories {
		if !cfg.ModuleIsEnabled(factory.Name) {
			log.Infof("module %s disabled", factory.Name)
			continue
		}

		module, err := factory.Fn(cfg)

		// In case a module failed to be started, do not make the whole `system-probe` abort.
		// Let `system-probe` run the other modules.
		if err != nil {
			l.errors[factory.Name] = err
			log.Errorf("error creating module %s: %s", factory.Name, err)
			continue
		}

		subRouter, err := makeSubrouter(httpMux, string(factory.Name))
		if err != nil {
			l.errors[factory.Name] = err
			log.Errorf("error making router for module %s: %s", factory.Name, err)
			continue
		}

		if err = module.Register(subRouter); err != nil {
			l.errors[factory.Name] = err
			log.Errorf("error registering HTTP endpoints for module %s: %s", factory.Name, err)
			continue
		}

		l.routers[factory.Name] = subRouter
		l.modules[factory.Name] = module

		log.Infof("module %s started", factory.Name)
	}

	if !driver.IsNeeded() {
		// if running, shut it down
		log.Debug("system-probe module initialization complete, driver not needed, shutting down")

		// shut the driver down and optionally disable it, if closed source isn't allowed anymore
		if err := driver.ForceStop(); err != nil {
			log.Warnf("error stopping driver: %s", err)
		}
	}
	l.cfg = cfg
	if len(l.modules) == 0 {
		return errors.New("no module could be loaded")
	}

	go updateStats()
	return nil
}

func makeSubrouter(r *mux.Router, namespace string) (*Router, error) {
	if namespace == "" {
		return nil, errors.New("module name not set")
	}
	return NewRouter(r.PathPrefix("/" + namespace).Subrouter()), nil
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
		l.errors[factory.Name] = err
		return err
	}
	delete(l.errors, factory.Name)
	log.Infof("module %s restarted", factory.Name)

	currentRouter, ok := l.routers[factory.Name]
	if !ok {
		return fmt.Errorf("module %s does not have an associated router", factory.Name)
	}

	err = newModule.Register(currentRouter)
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
	start := time.Now()
	then := time.Now()
	now := time.Now()
	ticker := time.NewTicker(15 * time.Second)

	for {
		l.Lock()
		if l.closed {
			l.Unlock()
			return
		}

		l.stats = make(map[string]interface{})
		for name, module := range l.modules {
			l.stats[string(name)] = module.GetStats()
		}
		for name, err := range l.errors {
			l.stats[string(name)] = map[string]string{"Error": err.Error()}
		}

		l.stats["updated_at"] = now.Unix()
		l.stats["delta_seconds"] = now.Sub(then).Seconds()
		l.stats["uptime"] = now.Sub(start).String()
		l.Unlock()

		then = now
		now = <-ticker.C
	}
}
