// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module is the scaffolding for a system-probe module and the loader used upon start
package module

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
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

	statsUpdateTime  telemetry.Gauge
	statsUpdateCount telemetry.Counter
}

func (l *loader) forEachModule(fn func(name sysconfigtypes.ModuleName, mod types.SystemProbeModule)) {
	for name, mod := range l.modules {
		withModule(name, func() {
			fn(name, mod)
		})
	}
}

func (l *loader) configureTelemetry(tm telemetry.Component) {
	l.statsUpdateTime = tm.NewGauge("modules", "stats_update_time_seconds", []string{"module"}, "Time taken to update the stats, in seconds")
	l.statsUpdateCount = tm.NewCounter("modules", "stats_update_count", []string{"module"}, "Count of stats updates")
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
func Register(cfg *sysconfigtypes.Config, telemetry telemetry.Component, httpMux *mux.Router, modules []types.SystemProbeModuleComponent, rcclient rcclient.Component) error {
	var enabledModules []types.SystemProbeModuleComponent
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

	l.configureTelemetry(telemetry)

	l.stats = make(map[string]interface{})
	l.forEachModule(func(name sysconfigtypes.ModuleName, mod types.SystemProbeModule) {
		go updateModuleStats(name, mod)
	})
	go updateGlobalStats()

	return nil
}

// GetStats returns the stats from all modules, namespaced by their names
func GetStats() map[string]interface{} {
	l.Lock()
	defer l.Unlock()

	// Copy the stats map to avoid race conditions
	return maps.Clone(l.stats)
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

// IsClosed returns true if the loader is closed, thread-safe
func (l *loader) IsClosed() bool {
	l.Lock()
	defer l.Unlock()
	return l.closed
}

func updateModuleStats(name sysconfigtypes.ModuleName, mod types.SystemProbeModule) {
	nameStr := string(name)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		if l.IsClosed() {
			return
		}

		startUpdateTs := time.Now()
		stats := mod.GetStats()
		updateTimeSeconds := time.Since(startUpdateTs).Seconds()
		l.statsUpdateTime.Set(updateTimeSeconds, nameStr)
		l.statsUpdateCount.Inc(nameStr)

		l.Lock()
		l.stats[nameStr] = stats
		l.Unlock()

		<-ticker.C
	}
}

func updateGlobalStats() {
	start := time.Now()
	lastUpdate := start
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		if l.IsClosed() {
			return
		}

		l.Lock()
		for name, err := range l.errors {
			l.stats[string(name)] = map[string]string{"Error": err.Error()}
		}

		l.stats["updated_at"] = time.Now().Unix()
		l.stats["delta_seconds"] = time.Since(lastUpdate).Seconds()
		l.stats["uptime"] = time.Since(start).String()
		l.Unlock()

		lastUpdate = time.Now()
		<-ticker.C
	}
}
