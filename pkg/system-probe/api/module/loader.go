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

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var l *loader

func init() {
	l = &loader{
		modules: make(map[sysconfigtypes.ModuleName]types.SystemProbeModule),
		errors:  make(map[sysconfigtypes.ModuleName]error),
		routers: make(map[sysconfigtypes.ModuleName]types.SystemProbeRouter),
	}
}

// loader is responsible for managing the lifecycle of each api.Module, which includes:
// * Module initialization;
// * Module termination;
// * Module telemetry consolidation;
type loader struct {
	sync.Mutex
	modules map[sysconfigtypes.ModuleName]types.SystemProbeModule
	errors  map[sysconfigtypes.ModuleName]error
	stats   map[string]interface{}
	cfg     *sysconfigtypes.Config
	routers map[sysconfigtypes.ModuleName]types.SystemProbeRouter
	closed  bool
}

func (l *loader) forEachModule(fn func(name sysconfigtypes.ModuleName, mod types.SystemProbeModule)) {
	for name, mod := range l.modules {
		withModule(name, func() {
			fn(name, mod)
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
func Register(cfg *sysconfigtypes.Config, httpMux *mux.Router, modules []types.SystemProbeModuleComponent, rcclient rcclient.Component) error {
	var enabledModules []types.SystemProbeModuleComponent
	// TODO can we filter out disabled modules before this?
	for _, mod := range modules {
		if !cfg.ModuleIsEnabled(mod.Name()) {
			log.Infof("module %s disabled", mod.Name())
			continue
		}
		enabledModules = append(enabledModules, mod)
	}

	if err := preRegister(cfg, rcclient, enabledModules); err != nil {
		return fmt.Errorf("error in pre-register hook: %w", err)
	}

	for _, mod := range enabledModules {
		var err error
		var module types.SystemProbeModule
		withModule(mod.Name(), func() {
			module, err = mod.Create()
		})

		// In case a module failed to be started, do not make the whole `system-probe` abort.
		// Let `system-probe` run the other modules.
		if err != nil {
			l.errors[mod.Name()] = err
			log.Errorf("error creating module %s: %s", mod.Name(), err)
			continue
		}

		subRouter := NewRouter(string(mod.Name()), httpMux)
		if err = module.Register(subRouter); err != nil {
			l.errors[mod.Name()] = err
			log.Errorf("error registering HTTP endpoints for module %s: %s", mod.Name(), err)
			continue
		}

		l.routers[mod.Name()] = subRouter
		l.modules[mod.Name()] = module

		log.Infof("module %s started", mod.Name())
	}

	if err := postRegister(cfg, enabledModules); err != nil {
		return fmt.Errorf("error in post-register hook: %w", err)
	}

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
func RestartModule(mod types.SystemProbeModuleComponent) error {
	l.Lock()
	defer l.Unlock()

	if l.closed {
		return errors.New("can't restart module because system-probe is shutting down")
	}

	currentModule := l.modules[mod.Name()]
	if currentModule == nil {
		return fmt.Errorf("module %s is not running", mod.Name())
	}
	currentRouter, ok := l.routers[mod.Name()]
	if !ok {
		return fmt.Errorf("module %s does not have an associated router", mod.Name())
	}

	var newModule types.SystemProbeModule
	var err error
	withModule(mod.Name(), func() {
		currentRouter.Unregister()
		currentModule.Close()
		newModule, err = mod.Create()
	})
	if err != nil {
		l.errors[mod.Name()] = err
		return err
	}
	delete(l.errors, mod.Name())
	log.Infof("module %s restarted", mod.Name())

	err = newModule.Register(currentRouter)
	if err != nil {
		return err
	}

	l.modules[mod.Name()] = newModule
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
	l.forEachModule(func(name sysconfigtypes.ModuleName, mod types.SystemProbeModule) {
		currentRouter, ok := l.routers[name]
		if ok {
			currentRouter.Unregister()
		}
		mod.Close()
	})
}

func updateStats() {
	start := time.Now()
	then := time.Now()
	now := time.Now()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		l.Lock()
		if l.closed {
			l.Unlock()
			return
		}

		l.stats = make(map[string]interface{})
		l.forEachModule(func(name sysconfigtypes.ModuleName, mod types.SystemProbeModule) {
			l.stats[string(name)] = mod.GetStats()
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
