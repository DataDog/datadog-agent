// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event_monitor

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	statsdPoolSize = 64
)

var (
	// allowedEventTypes defines allowed event type for subscribers
	allowedEventTypes = []model.EventType{model.ForkEventType, model.ExecEventType, model.ExecEventType}
)

// EventMonitor represents the system-probe module for runtime monitoring
type EventMonitor struct {
	sync.RWMutex
	Probe *probe.Probe

	Config       *sysconfig.Config
	StatsdClient statsd.ClientInterface
	GRPCServer   *grpc.Server

	// internals
	eventModules []EventModule
	netListener  net.Listener
	wg           sync.WaitGroup
	// TODO should be remove after migration to a common section
	secconfig *config.Config
}

type EventModule interface {
	ID() string
	Start() error
	Stop()
}

// EventTypeHandler event type based handler
type EventTypeHandler interface {
	probe.EventHandler
}

// Register the runtime security agent module
func (m *EventMonitor) Register(_ *module.Router) error {
	if err := m.Init(); err != nil {
		return err
	}

	return m.Start()
}

// AddEventTypeHandler register an event handler
func (m *EventMonitor) AddEventTypeHandler(eventType model.EventType, handler EventTypeHandler) error {
	if !slices.Contains(allowedEventTypes, eventType) {
		return errors.New("event type not allowed")
	}

	m.Probe.AddEventHandler(eventType, handler)

	return nil
}

// RegisterEventModule register an event module
func (m *EventMonitor) RegisterEventModule(em EventModule) {
	m.eventModules = append(m.eventModules, em)
}

// Init initializes the module
func (m *EventMonitor) Init() error {
	// force socket cleanup of previous socket not cleanup
	os.Remove(m.secconfig.SocketPath)

	// initialize the eBPF manager and load the programs and maps in the kernel. At this stage, the probes are not
	// running yet.
	if err := m.Probe.Init(); err != nil {
		return fmt.Errorf("failed to init probe: %w", err)
	}

	return nil
}

// Start the module
func (m *EventMonitor) Start() error {
	ln, err := net.Listen("unix", m.secconfig.SocketPath)
	if err != nil {
		return fmt.Errorf("unable to register security runtime module: %w", err)
	}
	if err := os.Chmod(m.secconfig.SocketPath, 0700); err != nil {
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

	// start event modules
	for _, em := range m.eventModules {
		if err := em.Start(); err != nil {
			log.Errorf("unable to start %s : %v", em.ID(), err)
		}
	}

	return nil
}

// Close the module
func (m *EventMonitor) Close() {
	// stop event modules
	for _, em := range m.eventModules {
		em.Stop()
	}

	if m.GRPCServer != nil {
		m.GRPCServer.Stop()
	}

	if m.netListener != nil {
		m.netListener.Close()
		os.Remove(m.secconfig.SocketPath)
	}

	m.wg.Wait()

	// all the go routines should be stopped now we can safely call close the probe and remove the eBPF programs
	m.Probe.Close()
}

// SendStats send stats
func (m *EventMonitor) SendStats() {
	// TODO
}

// GetStats returns statistics about the module
func (m *EventMonitor) GetStats() map[string]interface{} {
	debug := map[string]interface{}{}

	if m.Probe != nil {
		debug["probe"] = m.Probe.GetDebugStats()
	} else {
		debug["probe"] = "not_running"
	}

	return debug
}

func getStatdClient(config *config.Config) (statsd.ClientInterface, error) {
	statsdAddr := os.Getenv("STATSD_URL")
	if statsdAddr == "" {
		statsdAddr = config.StatsdAddr
	}

	return statsd.New(statsdAddr, statsd.WithBufferPoolSize(statsdPoolSize))
}

// NewModule instantiates a runtime security system-probe module
func NewEventMonitor(sysProbeConfig *sysconfig.Config) (*EventMonitor, error) {
	// TODO move probe config parameter to a common place
	config, err := config.NewConfig(sysProbeConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid event monitoring configuration: %w", err)
	}

	statsdClient, err := getStatdClient(config)
	if err != nil {
		return nil, err
	}

	probe, err := probe.NewProbe(config, statsdClient)
	if err != nil {
		return nil, err
	}

	return &EventMonitor{
		Config:       sysProbeConfig,
		Probe:        probe,
		StatsdClient: statsdClient,
		GRPCServer:   grpc.NewServer(),
		secconfig:    config,
	}, nil
}
