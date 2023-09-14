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
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

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

func (l *loader) forEachModule(fn func(name string, mod Module)) {
	for name, mod := range l.modules {
		withModule(name, func() {
			fn(string(name), mod)
		})
	}
}

func withModule(name config.ModuleName, fn func()) {
	pprof.Do(context.Background(), pprof.Labels("module", string(name)), func(_ context.Context) {
		fn()
	})
}

// Register a set of modules, which involves:
// * Initialization using the provided Factory;
// * Registering the HTTP endpoints of each module;
// * Register the gRPC server;
func Register(cfg *config.Config, httpMux *mux.Router, grpcServer *grpc.Server, factories []Factory) error {
	if err := driver.Init(cfg); err != nil {
		log.Warnf("Failed to load driver subsystem %v", err)
	}

	for _, factory := range factories {
		if !cfg.ModuleIsEnabled(factory.Name) {
			log.Infof("module %s disabled", factory.Name)
			continue
		}

		var err error
		var module Module
		withModule(factory.Name, func() {
			module, err = factory.Fn(cfg)
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

		if grpcServer != nil {
			if err = module.RegisterGRPC(&systemProbeGRPCServer{sr: grpcServer, ns: factory.Name}); err != nil {
				l.errors[factory.Name] = err
				log.Errorf("error registering grpc endpoints for module %s: %s", factory.Name, err)
				continue
			}
		}

		l.routers[factory.Name] = subRouter
		l.modules[factory.Name] = module

		log.Infof("module %s started", factory.Name)
	}

	if !driver.IsNeeded() {
		// if running, shut it down
		log.Debug("Shutting down the driver.  Upon successful initialization, it was not needed by the current configuration.")

		// shut the driver down and  disable it
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
	return NewRouter(namespace, r), nil
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
		newModule, err = factory.Fn(l.cfg)
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

type systemProbeGRPCServer struct {
	sr grpc.ServiceRegistrar
	ns config.ModuleName
}

func (s *systemProbeGRPCServer) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	modName := NameFromGRPCServiceName(desc.ServiceName)
	if modName != string(s.ns) {
		panic(fmt.Sprintf("module name `%s` from service name `%s` does not match `%s`", modName, desc.ServiceName, s.ns))
	}
	s.sr.RegisterService(desc, impl)
}

// NameFromGRPCServiceName extracts a system-probe module name from the gRPC service name.
// It expects a prefix of `datadog.agent.systemprobe.` and then the pascal cased version of the module name.
func NameFromGRPCServiceName(service string) string {
	prefix := "datadog.agent.systemprobe."
	if !strings.HasPrefix(service, prefix) {
		return ""
	}
	s := strings.TrimPrefix(service, prefix)
	// we are expecting a pascal case service name, so convert it to snake case to match system-probe module names
	return toSnakeCase(s)
}

func toSnakeCase(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				sb.WriteRune('_')
			}
			sb.WriteRune(unicode.ToLower(r))
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
