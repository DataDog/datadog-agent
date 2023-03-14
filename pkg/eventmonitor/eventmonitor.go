// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventmonitor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

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

var (
	// allowedEventTypes defines allowed event type for subscribers
	allowedEventTypes = []model.EventType{model.ForkEventType, model.ExecEventType, model.ExitEventType}
)

// EventMonitor represents the system-probe module for runtime monitoring
type EventMonitor struct {
	sync.RWMutex
	Probe *probe.Probe

	Config       *sysconfig.Config
	StatsdClient statsd.ClientInterface
	GRPCServer   *grpc.Server

	// internals
	ctx            context.Context
	cancelFnc      context.CancelFunc
	sendStatsChan  chan chan bool
	eventConsumers []EventConsumer
	netListener    net.Listener
	wg             sync.WaitGroup
	// TODO should be remove after migration to a common section
	secconfig *config.Config
}

// EventConsumer defines an event consumer
type EventConsumer interface {
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

	return m.Probe.AddEventHandler(eventType, handler)
}

// RegisterEventConsumer register an event module
func (m *EventMonitor) RegisterEventConsumer(consumer EventConsumer) {
	m.eventConsumers = append(m.eventConsumers, consumer)
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

	// start event consumers
	for _, em := range m.eventConsumers {
		if err := em.Start(); err != nil {
			log.Errorf("unable to start %s : %v", em.ID(), err)
		}
	}

	m.wg.Add(1)
	go m.statsSender()

	return nil
}

// Close the module
func (m *EventMonitor) Close() {
	// stop event consumers
	for _, em := range m.eventConsumers {
		em.Stop()
	}

	if m.GRPCServer != nil {
		m.GRPCServer.Stop()
	}

	if m.netListener != nil {
		m.netListener.Close()
		os.Remove(m.secconfig.SocketPath)
	}

	m.cancelFnc()
	m.wg.Wait()

	// all the go routines should be stopped now we can safely call close the probe and remove the eBPF programs
	m.Probe.Close()
}

// SendStats send stats
func (m *EventMonitor) SendStats() {
	ackChan := make(chan bool, 1)
	m.sendStatsChan <- ackChan
	<-ackChan
}

func (m *EventMonitor) sendStats() {
	if err := m.Probe.SendStats(); err != nil {
		seclog.Debugf("failed to send probe stats: %s", err)
	}
}

func (m *EventMonitor) statsSender() {
	defer m.wg.Done()

	statsTicker := time.NewTicker(m.secconfig.StatsPollingInterval)
	defer statsTicker.Stop()

	for {
		select {
		case ackChan := <-m.sendStatsChan:
			m.sendStats()
			ackChan <- true
		case <-statsTicker.C:
			m.sendStats()
		case <-m.ctx.Done():
			return
		}
	}
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

// NewModule instantiates a runtime security system-probe module
func NewEventMonitor(sysProbeConfig *sysconfig.Config, statsdClient statsd.ClientInterface, probeOpts probe.Opts) (*EventMonitor, error) {
	secconfig, err := config.NewConfig(sysProbeConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid event monitoring configuration: %w", err)
	}

	probe, err := probe.NewProbe(secconfig, probeOpts)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	return &EventMonitor{
		Config:       sysProbeConfig,
		Probe:        probe,
		StatsdClient: statsdClient,
		GRPCServer:   grpc.NewServer(),

		ctx:           ctx,
		cancelFnc:     cancelFnc,
		sendStatsChan: make(chan chan bool, 1),
		secconfig:     secconfig,
	}, nil
}
