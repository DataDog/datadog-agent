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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var l *loader

func init() {
	l = &loader{
		modules:    make(map[sysconfigtypes.ModuleName]Module),
		errors:     make(map[sysconfigtypes.ModuleName]error),
		routers:    make(map[sysconfigtypes.ModuleName]*Router),
		moduleStop: make(map[sysconfigtypes.ModuleName]chan struct{}),
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

	// httpMux is retained from Register so modules enabled after boot can mount
	// their routes on the running server.
	httpMux *http.ServeMux
	// moduleStop holds the stop channel for each module's stats goroutine, so it
	// can be torn down when a single module is disabled.
	moduleStop map[sysconfigtypes.ModuleName]chan struct{}

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
func Register(cfg *sysconfigtypes.Config, httpMux *http.ServeMux, factories []*Factory, rcclient rcclient.Component, deps FactoryDependencies) error {
	var enabledModulesFactories []*Factory
	for _, factory := range factories {
		if !cfg.ModuleIsEnabled(factory.Name) {
			log.Infof("module %s disabled", factory.Name)
			continue
		}
		enabledModulesFactories = append(enabledModulesFactories, factory)
	}

	if err := preRegister(cfg, rcclient, enabledModulesFactories); err != nil {
		return fmt.Errorf("error in pre-register hook: %w", err)
	}

	for _, factory := range enabledModulesFactories {
		var err error
		var module Module
		withModule(factory.Name, func() {
			module, err = factory.Fn(cfg, deps)
		})

		// In case a module failed to be started, do not make the whole `system-probe` abort.
		// Let `system-probe` run the other modules.
		if err != nil {
			l.moduleError(factory.Name, err)
			log.Errorf("error creating module %s: %s", factory.Name, err)
			continue
		}

		subRouter := NewRouter(string(factory.Name), httpMux)
		if err = module.Register(subRouter); err != nil {
			l.moduleError(factory.Name, err)
			log.Errorf("error registering HTTP endpoints for module %s: %s", factory.Name, err)
			continue
		}

		l.registerModule(factory.Name, module, subRouter)
		log.Infof("module %s started", factory.Name)
	}

	if err := postRegister(cfg, enabledModulesFactories); err != nil {
		return fmt.Errorf("error in post-register hook: %w", err)
	}

	l.cfg = cfg
	l.httpMux = httpMux
	if len(l.modules) == 0 {
		return errors.New("no module could be loaded")
	}

	l.configureTelemetry(deps.Telemetry)

	l.stats = make(map[string]any)
	l.forEachModule(func(name sysconfigtypes.ModuleName, mod Module) {
		l.startModuleStats(name, mod)
	})
	go updateGlobalStats()

	return nil
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

// RestartModule triggers a module restart
func RestartModule(factory *Factory, deps FactoryDependencies) error {
	l.Lock()
	defer l.Unlock()

	if l.closed {
		return errors.New("can't restart module because system-probe is shutting down")
	}

	currentModule := l.modules[factory.Name]
	if currentModule == nil {
		return fmt.Errorf("module %s is not running", factory.Name)
	}
	currentRouter, ok := l.routers[factory.Name]
	if !ok {
		return fmt.Errorf("module %s does not have an associated router", factory.Name)
	}

	var newModule Module
	var err error
	withModule(factory.Name, func() {
		currentRouter.Unregister()
		currentModule.Close()
		delete(l.modules, factory.Name)
		newModule, err = factory.Fn(l.cfg, deps)
	})
	if err != nil {
		l.errors[factory.Name] = err
		delete(l.routers, factory.Name)
		return err
	}
	delete(l.errors, factory.Name)
	log.Infof("module %s restarted", factory.Name)

	err = newModule.Register(currentRouter)
	if err != nil {
		return err
	}

	l.modules[factory.Name] = newModule
	return nil
}

// EnableModule constructs and registers a module that is not currently running.
// It is a no-op if the module is already loaded. Unlike RestartModule, it can
// start a module that was not enabled at boot, which is how a module is turned
// on at runtime.
func EnableModule(factory *Factory, deps FactoryDependencies) error {
	l.Lock()
	defer l.Unlock()

	if l.closed {
		return errors.New("can't enable module because system-probe is shutting down")
	}
	if _, running := l.modules[factory.Name]; running {
		return nil
	}

	// Reuse the router from a previous enable if one exists; creating a second
	// router for the same name would re-register the prefix on the mux and panic.
	router, ok := l.routers[factory.Name]
	if !ok {
		if l.httpMux == nil {
			return fmt.Errorf("can't enable module %s before the api server is started", factory.Name)
		}
		router = NewRouter(string(factory.Name), l.httpMux)
		l.routers[factory.Name] = router
	}

	var newModule Module
	var err error
	withModule(factory.Name, func() {
		newModule, err = factory.Fn(l.cfg, deps)
	})
	if err != nil {
		l.errors[factory.Name] = err
		return err
	}
	if err := newModule.Register(router); err != nil {
		l.errors[factory.Name] = err
		return err
	}

	delete(l.errors, factory.Name)
	l.modules[factory.Name] = newModule
	l.startModuleStats(factory.Name, newModule)
	log.Infof("module %s enabled", factory.Name)
	return nil
}

// DisableModule unregisters and closes a running module. It is a no-op if the
// module is not loaded. The router is retained so the module can be re-enabled
// later without re-registering its prefix on the HTTP mux.
func DisableModule(name sysconfigtypes.ModuleName) error {
	l.Lock()
	defer l.Unlock()

	if l.closed {
		return errors.New("can't disable module because system-probe is shutting down")
	}
	mod, running := l.modules[name]
	if !running {
		return nil
	}

	withModule(name, func() {
		if router, ok := l.routers[name]; ok {
			router.Unregister()
		}
		mod.Close()
	})

	if stop, ok := l.moduleStop[name]; ok {
		close(stop)
		delete(l.moduleStop, name)
	}
	delete(l.modules, name)
	delete(l.errors, name)
	delete(l.stats, string(name))
	log.Infof("module %s disabled", name)
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
	l.forEachModule(func(name sysconfigtypes.ModuleName, mod Module) {
		currentRouter, ok := l.routers[name]
		if ok {
			currentRouter.Unregister()
		}
		mod.Close()
		delete(l.modules, name)
	})
}

// IsClosed returns true if the loader is closed, thread-safe
func (l *loader) IsClosed() bool {
	l.Lock()
	defer l.Unlock()
	return l.closed
}

// startModuleStats launches the stats-polling goroutine for a module and records
// its stop channel so it can be torn down independently when the module is
// disabled. Callers either hold l or run during boot registration. Telemetry
// must already be configured.
func (l *loader) startModuleStats(name sysconfigtypes.ModuleName, mod Module) {
	if l.statsUpdateTime == nil {
		return
	}
	stop := make(chan struct{})
	l.moduleStop[name] = stop
	go updateModuleStats(name, mod, stop)
}

func updateModuleStats(name sysconfigtypes.ModuleName, mod Module, stop <-chan struct{}) {
	nameStr := string(name)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		if l.IsClosed() {
			return
		}
		select {
		case <-stop:
			return
		default:
		}

		startUpdateTs := time.Now()
		stats := mod.GetStats()
		updateTimeSeconds := time.Since(startUpdateTs).Seconds()
		l.statsUpdateTime.Set(updateTimeSeconds, nameStr)
		l.statsUpdateCount.Inc(nameStr)

		l.Lock()
		l.stats[nameStr] = stats
		l.Unlock()

		select {
		case <-ticker.C:
		case <-stop:
			return
		}
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
