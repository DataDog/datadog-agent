// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

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
	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
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
	allowedEventTypes = []model.EventType{model.ForkEventType, model.ExecEventType, model.ExitEventType}
)

// Opts defines options that can be used for the eventmonitor
type Opts struct {
	ProbeOpts    probe.Opts
	StatsdClient statsd.ClientInterface
}

// EventMonitor represents the system-probe module for kernel event monitoring
type EventMonitor struct {
	sync.RWMutex
	Probe *probe.Probe

	Config       *config.Config
	StatsdClient statsd.ClientInterface
	GRPCServer   *grpc.Server

	// internals
	ctx            context.Context
	cancelFnc      context.CancelFunc
	sendStatsChan  chan chan bool
	eventConsumers []EventConsumer
	netListener    net.Listener
	wg             sync.WaitGroup
}

var _ module.Module = &EventMonitor{}

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

// Register the event monitoring module
func (m *EventMonitor) Register(_ *module.Router) error {
	if err := m.Init(); err != nil {
		return err
	}

	return m.Start()
}

// AddEventTypeHandler registers an event handler
func (m *EventMonitor) AddEventTypeHandler(eventType model.EventType, handler EventTypeHandler) error {
	if !slices.Contains(allowedEventTypes, eventType) {
		return errors.New("event type not allowed")
	}

	return m.Probe.AddEventHandler(eventType, handler)
}

// RegisterEventConsumer registers an event consumer
func (m *EventMonitor) RegisterEventConsumer(consumer EventConsumer) {
	m.eventConsumers = append(m.eventConsumers, consumer)
}

// Init initializes the module
func (m *EventMonitor) Init() error {
	// force socket cleanup of previous socket not cleanup
	os.Remove(m.Config.SocketPath)

	// initialize the eBPF manager and load the programs and maps in the kernel. At this stage, the probes are not
	// running yet.
	if err := m.Probe.Init(); err != nil {
		return fmt.Errorf("failed to init probe: %w", err)
	}

	return nil
}

// Start the module
func (m *EventMonitor) Start() error {
	ln, err := net.Listen("unix", m.Config.SocketPath)
	if err != nil {
		return fmt.Errorf("unable to register event monitoring module: %w", err)
	}

	if err := os.Chmod(m.Config.SocketPath, 0700); err != nil {
		return fmt.Errorf("unable to register event monitoring module: %w", err)
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
			log.Errorf("unable to start %s event consumer: %v", em.ID(), err)
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
		os.Remove(m.Config.SocketPath)
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

	statsTicker := time.NewTicker(m.Probe.StatsPollingInterval())
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

// NewEventMonitor instantiates an event monitoring system-probe module
func NewEventMonitor(config *config.Config, secconfig *secconfig.Config, opts Opts) (*EventMonitor, error) {
	if opts.StatsdClient == nil {
		statsdClient, err := getStatsdClient(config)
		if err != nil {
			log.Info("Unable to init statsd client")
			return nil, module.ErrNotEnabled
		}
		opts.StatsdClient = statsdClient
	}

	if opts.ProbeOpts.StatsdClient == nil {
		opts.ProbeOpts.StatsdClient = opts.StatsdClient
	}

	probe, err := probe.NewProbe(secconfig, opts.ProbeOpts)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	return &EventMonitor{
		Config:       config,
		Probe:        probe,
		StatsdClient: opts.StatsdClient,
		GRPCServer:   grpc.NewServer(),

		ctx:           ctx,
		cancelFnc:     cancelFnc,
		sendStatsChan: make(chan chan bool, 1),
	}, nil
}

func getStatsdClient(cfg *config.Config) (statsd.ClientInterface, error) {
	statsdAddr := os.Getenv("STATSD_URL")
	if statsdAddr == "" {
		statsdAddr = cfg.StatsdAddr
	}

	return statsd.New(statsdAddr, statsd.WithBufferPoolSize(statsdPoolSize))
}
