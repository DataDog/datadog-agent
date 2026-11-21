// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	stats   map[string]any
	cfg     *sysconfigtypes.Config
	routers map[sysconfigtypes.ModuleName]*Router
	closed  bool

	statsUpdateTime  telemetry.Gauge
	statsUpdateCount telemetry.Counter
}

func (l *loader) forEachModule(fn func(name sysconfigtypes.ModuleName, mod Module)) {
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
func Register(cfg *sysconfigtypes.Config, httpMux *http.ServeMux, factories []*Factory, rcclient rcclient.Component, deps FactoryDependencies, startupPhase *logging.StartupPhase) error {
	phase := startupPhase.StartPhase("modules.select_enabled")
	var enabledModulesFactories []*Factory
	for _, factory := range factories {
		if !cfg.ModuleIsEnabled(factory.Name) {
			log.Infof("module %s disabled", factory.Name)
			continue
		}
		enabledModulesFactories = append(enabledModulesFactories, factory)
	}
	phase.Finish(nil)

	phase = startupPhase.StartPhase("modules.preRegister")
	err := preRegister(cfg, rcclient, deps.Telemetry, enabledModulesFactories)
	phase.Finish(err)
	if err != nil {
		return fmt.Errorf("error in pre-register hook: %w", err)
	}

	for _, factory := range enabledModulesFactories {
		l.registerFactory(cfg, httpMux, factory, deps, startupPhase)
	}

	phase = startupPhase.StartPhase("modules.postRegister")
	err = postRegister(cfg, enabledModulesFactories)
	phase.Finish(err)
	if err != nil {
		return fmt.Errorf("error in post-register hook: %w", err)
	}

	l.Lock()
	l.cfg = cfg
	if len(l.modules) == 0 {
		l.Unlock()
		return errors.New("no module could be loaded")
	}
	l.Unlock()

	phase = startupPhase.StartPhase("modules.configure_telemetry")
	l.configureTelemetry(deps.Telemetry)
	phase.Finish(nil)

	phase = startupPhase.StartPhase("modules.stats.launch")
	l.Lock()
	l.stats = make(map[string]any)
	l.forEachModule(func(name sysconfigtypes.ModuleName, mod Module) {
		go updateModuleStats(name, mod)
	})
	l.Unlock()
	go updateGlobalStats()
	phase.Finish(nil)

	return nil
}

func (l *loader) registerFactory(cfg *sysconfigtypes.Config, httpMux *http.ServeMux, factory *Factory, deps FactoryDependencies, startupPhase *logging.StartupPhase) {
	// Factory names are fixed module identifiers, not configuration values.
	phase := startupPhase.StartPhase("module." + string(factory.Name) + ".create")
	var err error
	var module Module
	withModule(factory.Name, func() {
		module, err = factory.Fn(cfg, deps)
	})
	phase.Finish(err)

	// A failed module must not prevent other modules from starting.
	if err != nil {
		l.moduleError(factory.Name, err)
		log.Errorf("error creating module %s: %s", factory.Name, err)
		return
	}

	phase = startupPhase.StartPhase("module." + string(factory.Name) + ".register")
	subRouter := NewRouter(string(factory.Name), httpMux)
	err = module.Register(subRouter)
	phase.Finish(err)
	if err != nil {
		l.moduleError(factory.Name, err)
		log.Errorf("error registering HTTP endpoints for module %s: %s", factory.Name, err)
		return
	}

	l.registerModule(factory.Name, module, subRouter)
	log.Infof("module %s started", factory.Name)
}

func (l *loader) registerModule(name sysconfigtypes.ModuleName, module Module, subRouter *Router) {
	l.Lock()
	defer l.Unlock()
	l.routers[name] = subRouter
	l.modules[name] = module
}

func (l *loader) moduleError(name sysconfigtypes.ModuleName, err error) {
	l.Lock()
	defer l.Unlock()
	l.errors[name] = err
}

// IsLoaded returns whether the named module has successfully loaded
func IsLoaded(name sysconfigtypes.ModuleName) bool {
	l.Lock()
	defer l.Unlock()

	_, found := l.modules[name]
	return found
}

// GetStats returns the stats from all modules, namespaced by their names
func GetStats() map[string]any {
	l.Lock()
	defer l.Unlock()

	// Copy the stats map to avoid race conditions
	return maps.Clone(l.stats)
}

// Close each registered module
func Close() {
	CloseWithTrace(nil)
}

// CloseWithTrace records each module teardown without changing the close order.
func CloseWithTrace(trace *logging.StartupPhase) {
	l.Lock()
	defer l.Unlock()

	if l.closed {
		return
	}

	l.closed = true
	l.forEachModule(func(name sysconfigtypes.ModuleName, mod Module) {
		currentRouter, ok := l.routers[name]
		if ok {
			phase := trace.StartPhase("module." + string(name) + ".unregister")
			currentRouter.Unregister()
			phase.Finish(nil)
		}
		phase := trace.StartPhase("module." + string(name) + ".close")
		if traced, ok := mod.(interface{ CloseWithTrace(*logging.StartupPhase) }); ok {
			traced.CloseWithTrace(phase)
		} else {
			mod.Close()
		}
		phase.Finish(nil)
		delete(l.modules, name)
	})
}

// IsClosed returns true if the loader is closed, thread-safe
func (l *loader) IsClosed() bool {
	l.Lock()
	defer l.Unlock()
	return l.closed
}

func updateModuleStats(name sysconfigtypes.ModuleName, mod Module) {
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
