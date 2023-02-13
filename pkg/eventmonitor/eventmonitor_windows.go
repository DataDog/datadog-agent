// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package eventmonitor

import (
	"context"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
	"net"
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

var (
	// allowedEventTypes defines allowed event type for subscribers
	allowedEventTypes = []model.EventType{model.ForkEventType, model.ExecEventType, model.ExitEventType}
)

// EventMonitor represents the system-probe module for runtime monitoring
type EventMonitor struct {
	sync.RWMutex

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

	// TODO

	return nil
}

// RegisterEventConsumer register an event module
func (m *EventMonitor) RegisterEventConsumer(consumer EventConsumer) {
	m.eventConsumers = append(m.eventConsumers, consumer)
}

// Init initializes the module
func (m *EventMonitor) Init() error {
	// force socket cleanup of previous socket not cleanup
	os.Remove(m.secconfig.SocketPath)

	// TODO

	return nil
}

// Start the module
func (m *EventMonitor) Start() error {

	// TODO

	return nil
}

// Close the module
func (m *EventMonitor) Close() {

	// TODO

}

// SendStats send stats
func (m *EventMonitor) SendStats() {

	// TODO

}

func (m *EventMonitor) sendStats() {

	// TODO

}

func (m *EventMonitor) statsSender() {

	// TODO

}

// GetStats returns statistics about the module
func (m *EventMonitor) GetStats() map[string]interface{} {
	debug := map[string]interface{}{}

	// TODO

	return debug
}

// NewModule instantiates a runtime security system-probe module
func NewEventMonitor(sysProbeConfig *sysconfig.Config, statsdClient statsd.ClientInterface) (*EventMonitor, error) {
	secconfig, err := config.NewConfig(sysProbeConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid event monitoring configuration: %w", err)
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	return &EventMonitor{
		Config:       sysProbeConfig,
		StatsdClient: statsdClient,
		GRPCServer:   grpc.NewServer(),

		ctx:           ctx,
		cancelFnc:     cancelFnc,
		sendStatsChan: make(chan chan bool, 1),
		secconfig:     secconfig,
	}, nil
}
