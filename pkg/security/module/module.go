// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

const (
	statsdPoolSize = 64
)

// Module represents the system-probe module for runtime monitoring
type Module struct {
	sync.RWMutex
	Probe        *sprobe.Probe
	Config       *sconfig.Config
	StatsdClient statsd.ClientInterface
	GRPCServer   *grpc.Server

	// internalls
	eventModules []EventModule
	netListener  net.Listener
	wg           sync.WaitGroup
}

// EventModuleCtor constructor of an event module
type EventModuleCtor func(_ *Module) (EventModule, error)

type EventModule interface {
	Start() error
	Stop()
	EventHanlders() map[model.EventType]sprobe.EventHandler
}

// Register the runtime security agent module
func (m *Module) Register(_ *module.Router) error {
	if err := m.Init(); err != nil {
		return err
	}

	return m.Start()
}

// RegisterEventModule register an event module, has to be called before start
func (m *Module) RegisterEventModule(ctor EventModuleCtor) error {
	em, err := ctor(m)
	if err != nil {
		return err
	}

	m.eventModules = append(m.eventModules, em)

	return nil
}

// Init initializes the module
func (m *Module) Init() error {
	// force socket cleanup of previous socket not cleanup
	os.Remove(m.Config.SocketPath)

	// initialize the eBPF manager and load the programs and maps in the kernel. At this stage, the probes are not
	// running yet.
	if err := m.Probe.Init(); err != nil {
		return fmt.Errorf("failed to init probe: %w", err)
	}

	// init handlers
	for _, module := range m.eventModules {
		handlers := module.EventHanlders()
		for eventType, handler := range handlers {
			m.Probe.AddEventHandler(eventType, handler)
		}
	}

	return nil
}

// Start the module
func (m *Module) Start() error {
	ln, err := net.Listen("unix", m.Config.SocketPath)
	if err != nil {
		return fmt.Errorf("unable to register security runtime module: %w", err)
	}
	if err := os.Chmod(m.Config.SocketPath, 0700); err != nil {
		return fmt.Errorf("unable to register security runtime module: %w", err)
	}

	m.netListener = ln

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		if err := m.GRPCServer.Serve(ln); err != nil {
			seclog.Errorf("error launching the grpc server: %v", err)
		}
	}()

	// setup the manager and its probes / perf maps
	if err := m.Probe.Setup(); err != nil {
		return fmt.Errorf("failed to setup probe: %w", err)
	}

	// fetch the current state of the system (example: mount points, running processes, ...) so that our user space
	// context is ready when we start the probes
	if err := m.Probe.Snapshot(); err != nil {
		return err
	}

	if err := m.Probe.Start(); err != nil {
		return err
	}

	// start all the event modules
	for _, em := range m.eventModules {
		em.Start()
	}

	return nil
}

// Close the module
func (m *Module) Close() {
	if m.GRPCServer != nil {
		m.GRPCServer.Stop()
	}

	if m.netListener != nil {
		m.netListener.Close()
		os.Remove(m.Config.SocketPath)
	}

	m.wg.Wait()

	// all the go routines should be stopped now we can safely call close the probe and remove the eBPF programs
	m.Probe.Close()
}

// SendStats send stats
func (m *Module) SendStats() {
	// TODO
}

// GetStats returns statistics about the module
func (m *Module) GetStats() map[string]interface{} {
	debug := map[string]interface{}{}

	if m.Probe != nil {
		debug["probe"] = m.Probe.GetDebugStats()
	} else {
		debug["probe"] = "not_running"
	}

	return debug
}

func getStatdClient(cfg *sconfig.Config, opts ...Opts) (statsd.ClientInterface, error) {
	if len(opts) != 0 && opts[0].StatsdClient != nil {
		return opts[0].StatsdClient, nil
	}

	statsdAddr := os.Getenv("STATSD_URL")
	if statsdAddr == "" {
		statsdAddr = cfg.StatsdAddr
	}

	return statsd.New(statsdAddr, statsd.WithBufferPoolSize(statsdPoolSize))
}

// NewModule instantiates a runtime security system-probe module
func NewModule(cfg *sconfig.Config, opts ...Opts) (*Module, error) {
	statsdClient, err := getStatdClient(cfg, opts...)
	if err != nil {
		return nil, err
	}

	probe, err := sprobe.NewProbe(cfg, statsdClient)
	if err != nil {
		return nil, err
	}

	return &Module{
		Config:       cfg,
		Probe:        probe,
		StatsdClient: statsdClient,
		GRPCServer:   grpc.NewServer(),
	}, nil
}
