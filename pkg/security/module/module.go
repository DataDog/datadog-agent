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
	"github.com/DataDog/datadog-agent/pkg/security/events"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
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

	// handlers
	eventTypeHandlers       []EventTypeHandler
	customEventTypeHandlers []CustomEventTypeHandler
	ruleEventHandlers       []RuleEventHandler

	// internals
	netListener net.Listener
	wg          sync.WaitGroup
}

// EventHandler generic event handler
type EventHandler interface {
	// ID return an unique ID for this handler
	ID() string
}

// EventTypeHandler event type based handler
type EventTypeHandler interface {
	EventHandler
	EventTypes() []model.EventType
	HandleEvent(event *model.Event)
}

// CustomEventTypeHandler custom event type based handler
type CustomEventTypeHandler interface {
	EventHandler
	CustomEventTypes() []model.EventType
	HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent)
}

// RuleEventHandler rule event based handler
type RuleEventHandler interface {
	EventHandler
	HandleRuleEvent(rule *rules.Rule, event *model.Event)
}

// Register the runtime security agent module
func (m *Module) Register(_ *module.Router) error {
	if err := m.Init(); err != nil {
		return err
	}

	return m.Start()
}

func (m *Module) AddEventTypeHandler(handler EventTypeHandler) {
	m.eventTypeHandlers = append(m.eventTypeHandlers, handler)
}

func (m *Module) AddCustomEventTypeHandler(handler CustomEventTypeHandler) {
	m.customEventTypeHandlers = append(m.customEventTypeHandlers, handler)
}

func (m *Module) AddRuleEventHandler(handler RuleEventHandler) {
	m.ruleEventHandlers = append(m.ruleEventHandlers, handler)
}

// HandleEvent implements probe events
func (m *Module) HandleEvent(event *model.Event) {
	for _, handler := range m.eventTypeHandlers {
		kind := event.GetEventType()
		for _, evt := range handler.EventTypes() {
			if kind == evt {
				handler.HandleEvent(event)
			}
		}
	}
}

// HandleEvent implements probe events
func (m *Module) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	for _, handler := range m.customEventTypeHandlers {
		kind := event.GetEventType()
		for _, evt := range handler.CustomEventTypes() {
			if kind == evt {
				handler.HandleCustomEvent(rule, event)
			}
		}
	}
}

// AddRule add a rule
func (m *Module) AddRule(handler RuleEventHandler, ruleDef *rules.RuleDefinition) error {
	// TODO check that the handler has been already registered
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

	// init event type handlers
	m.Probe.AddEventHandler(model.UnknownEventType, m)
	m.Probe.AddCustomEventHandler(model.UnknownEventType, m)

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
