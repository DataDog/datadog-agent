// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package eventmonitor holds eventmonitor related files
package eventmonitor

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// allowedEventTypes defines allowed event type for consumers
	allowedEventTypes = []model.EventType{model.ForkEventType, model.ExecEventType, model.ExitEventType, model.TracerMemfdSealEventType}
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

	// internals
	ctx            context.Context
	cancelFnc      context.CancelFunc
	sendStatsChan  chan chan bool
	eventConsumers []EventConsumer
	wg             sync.WaitGroup

	cwsStatusProvider CWSStatusProvider
}

var _ module.Module = &EventMonitor{}

// Register the event monitoring module
func (m *EventMonitor) Register(_ *module.Router) error {
	if err := m.Init(); err != nil {
		return err
	}

	return m.Start()
}

// AddEventConsumerHandler registers an event handler
func (m *EventMonitor) AddEventConsumerHandler(consumer EventConsumerHandler) error {
	for _, eventType := range consumer.EventTypes() {
		if !slices.Contains(allowedEventTypes, eventType) {
			return fmt.Errorf("event type (%s) not allowed", eventType)
		}
	}

	return m.Probe.AddEventConsumer(consumer)
}

// RegisterEventConsumer registers an event consumer
func (m *EventMonitor) RegisterEventConsumer(consumer EventConsumer) {
	m.eventConsumers = append(m.eventConsumers, consumer)
}

// CWSStatusProvider defines an interface to get CWS status
type CWSStatusProvider interface {
	GetStatus(ctx context.Context) (*api.Status, error)
}

// SetCWSStatusProvider sets the CWS status provider
func (m *EventMonitor) SetCWSStatusProvider(provider CWSStatusProvider) {
	m.cwsStatusProvider = provider
}

// Init initializes the module
func (m *EventMonitor) Init() error {
	// initialize the eBPF manager and load the programs and maps in the kernel. At this stage, the probes are not
	// running yet.
	if err := m.Probe.Init(); err != nil {
		return fmt.Errorf("failed to init probe: %w", err)
	}

	return nil
}

// Start the module
func (m *EventMonitor) Start() error {
	// fetch the current state of the system (example: mount points, running processes, ...) so that our user space
	// context is ready when we start the probes
	if err := m.Probe.Snapshot(); err != nil {
		return err
	}

	// start event consumers
	for _, em := range m.eventConsumers {
		if err := em.Start(); err != nil {
			log.Errorf("unable to start %s event consumer: %v", em.ID(), err)
		}
	}

	if err := m.Probe.Start(); err != nil {
		return err
	}

	for _, em := range m.eventConsumers {
		if ppsem, ok := em.(EventConsumerPostProbeStartHandler); ok {
			if err := ppsem.PostProbeStart(); err != nil {
				log.Errorf("after probe start callback of %s failed: %v", em.ID(), err)
			}
		}
	}

	m.wg.Add(1)
	go m.statsSender()

	return nil
}

// Close the module
func (m *EventMonitor) Close() {
	// stop so that consumers won't receive events anymore
	m.Probe.Stop()

	// stop event consumers
	for _, em := range m.eventConsumers {
		em.Stop()
	}

	m.cancelFnc()
	m.wg.Wait()

	// all the go routines should be stopped now we can safely call close the probe and remove the eBPF programs
	if err := m.Probe.Close(); err != nil {
		seclog.Errorf("failed to close event monitor probe: %v", err)
	}
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

	if m.cwsStatusProvider != nil {
		cwsStatus, err := m.cwsStatusProvider.GetStatus(context.Background())
		if err != nil {
			debug["cws"] = fmt.Sprintf("failed to get CWS status: %v", err)
		} else {
			debug["cws"] = cwsStatus
		}
	}

	return debug
}

// NewEventMonitor instantiates an event monitoring system-probe module
func NewEventMonitor(config *config.Config, secconfig *secconfig.Config, hostname string, opts Opts) (*EventMonitor, error) {
	if opts.StatsdClient == nil {
		opts.StatsdClient = &statsd.NoOpClient{}
	}

	if opts.ProbeOpts.StatsdClient == nil {
		opts.ProbeOpts.StatsdClient = opts.StatsdClient
	}

	probe, err := probe.NewProbe(secconfig, hostname, opts.ProbeOpts)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	return &EventMonitor{
		Config:       config,
		Probe:        probe,
		StatsdClient: opts.StatsdClient,

		ctx:           ctx,
		cancelFnc:     cancelFnc,
		sendStatsChan: make(chan chan bool, 1),
	}, nil
}
