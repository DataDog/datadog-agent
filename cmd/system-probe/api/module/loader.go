// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"errors"
	"fmt"
	"runtime/pprof"
	"sync"
	"time"

	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/gorilla/mux"
)

var l *loader

func init() {
	l = &loader{
		modules: make(map[sysconfigtypes.ModuleName]Module),
		errors:  make(map[sysconfigtypes.ModuleName]error),
		routers: make(map[sysconfigtypes.ModuleName]*Router),
	}
}

// loader is responsible for managing the lifecycle of each api.Module, which includes:
// * Module initialization;
// * Module termination;
// * Module telemetry consolidation;
type loader struct {
	sync.Mutex
	modules map[sysconfigtypes.ModuleName]Module
	errors  map[sysconfigtypes.ModuleName]error
	stats   map[string]interface{}
	cfg     *sysconfigtypes.Config
	routers map[sysconfigtypes.ModuleName]*Router
	closed  bool
}

func (l *loader) forEachModule(fn func(name string, mod Module)) {
	for name, mod := range l.modules {
		withModule(name, func() {
			fn(string(name), mod)
		})
	}
}

func withModule(name sysconfigtypes.ModuleName, fn func()) {
	pprof.Do(context.Background(), pprof.Labels("module", string(name)), func(_ context.Context) {
		fn()
	})
}

// Register a set of modules, which involves:
// * Initialization using the provided Factory;
// * Registering the HTTP endpoints of each module;
// * Register the gRPC server;
func Register(cfg *sysconfigtypes.Config, httpMux *mux.Router, factories []Factory, wmeta optional.Option[workloadmeta.Component]) error {
	var enabledModulesFactories []Factory
	for _, factory := range factories {
		if !cfg.ModuleIsEnabled(factory.Name) {
			log.Infof("module %s disabled", factory.Name)
			continue
		}
		enabledModulesFactories = append(enabledModulesFactories, factory)
	}

	if err := preRegister(cfg, enabledModulesFactories); err != nil {
		return fmt.Errorf("error in pre-register hook: %w", err)
	}

	for _, factory := range enabledModulesFactories {
		var err error
		var module Module
		withModule(factory.Name, func() {
			module, err = factory.Fn(cfg, wmeta)
		})

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

	if err := postRegister(cfg, enabledModulesFactories); err != nil {
		return fmt.Errorf("error in post-register hook: %w", err)
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
	return NewRouter(namespace, r), nil
}

// GetStats returns the stats from all modules, namespaced by their names
func GetStats() map[string]interface{} {
	l.Lock()
	defer l.Unlock()
	return l.stats
}

// RestartModule triggers a module restart
func RestartModule(factory Factory, wmeta optional.Option[workloadmeta.Component]) error {
	l.Lock()
	defer l.Unlock()

	if l.closed {
		return fmt.Errorf("can't restart module because system-probe is shutting down")
	}

	currentModule := l.modules[factory.Name]
	if currentModule == nil {
		return fmt.Errorf("module %s is not running", factory.Name)
	}

	var newModule Module
	var err error
	withModule(factory.Name, func() {
		currentModule.Close()
		newModule, err = factory.Fn(l.cfg, wmeta)
	})
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

	if l.closed {
		return
	}

	l.closed = true
	l.forEachModule(func(_ string, mod Module) {
		mod.Close()
	})
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
		l.forEachModule(func(name string, mod Module) {
			l.stats[name] = mod.GetStats()
		})
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
