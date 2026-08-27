// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf && nvml

package gpu

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	nvidiaXidPrefixPattern = regexp.MustCompile(`(?i)\bNVRM:\s*Xid\b`)
	nvidiaPCIBusIDPattern  = regexp.MustCompile(`(?i)\bPCI:\s*([[:xdigit:]]{4}:[[:xdigit:]]{2}:[[:xdigit:]]{2})(?:\.[0-7])?`)
	nvidiaXidCodePattern   = regexp.MustCompile(`\)\s*:\s*([0-9]+)\b`)
	logLimit               = log.NewLimit(10)
)

type driverEventTelemetry struct {
	once          sync.Once
	dropped       telemetry.Counter
	parseFailures telemetry.Counter
	unresolvedPCI telemetry.Counter
}

var driverEventsTelemetryDefinitions driverEventTelemetry

func (t *driverEventTelemetry) init(component telemetry.Component) {
	t.once.Do(func() {
		t.dropped = component.NewCounter("gpu__driver_events", "dropped", nil, "Number of NVIDIA driver events dropped because the queue was full")
		t.parseFailures = component.NewCounter("gpu__driver_events", "parse_failures", []string{"reason"}, "Number of NVIDIA driver events that could not be parsed")
		t.unresolvedPCI = component.NewCounter("gpu__driver_events", "unresolved_pci", nil, "Number of NVIDIA driver events whose PCI address could not be resolved to a GPU UUID")
	})
}

var errDriverEventSubscriberStopped = errors.New("driver event subscriber stopped")

type driverEventReader interface {
	Records() <-chan kernel.KmsgRecord
	Errors() <-chan error
	Stop()
}

// DriverEventSubscriber reads NVIDIA Xid events from /dev/kmsg and associates them with physical GPU UUIDs.
type DriverEventSubscriber struct {
	reader       driverEventReader
	telemetry    *driverEventTelemetry
	timeResolver *ktime.Resolver

	events      chan model.DriverEvent
	deviceCache ddnvml.DeviceCache
	stopOnce    sync.Once
	done        chan struct{}
}

// DriverEventSubscriberConfig configures a DriverEventSubscriber.
type DriverEventSubscriberConfig struct {
	QueueSize int
}

// NewDriverEventSubscriber starts a reader for future NVIDIA Xid kernel messages.
func NewDriverEventSubscriber(component telemetry.Component, deviceCache ddnvml.DeviceCache, cfg DriverEventSubscriberConfig) (*DriverEventSubscriber, error) {
	if component == nil {
		return nil, fmt.Errorf("driver event telemetry component is nil")
	}
	if deviceCache == nil {
		return nil, fmt.Errorf("GPU device cache is nil")
	}
	if cfg.QueueSize <= 0 {
		return nil, fmt.Errorf("driver event queue size must be positive, got %d", cfg.QueueSize)
	}

	timeResolver, err := ktime.NewResolver()
	if err != nil {
		return nil, fmt.Errorf("create kernel time resolver: %w", err)
	}

	reader, err := kernel.NewKmsgReader(component, isNvidiaXidMessage)
	if err != nil {
		return nil, fmt.Errorf("create kmsg reader: %w", err)
	}

	driverEventsTelemetryDefinitions.init(component)
	subscriber := &DriverEventSubscriber{
		reader:       reader,
		telemetry:    &driverEventsTelemetryDefinitions,
		timeResolver: timeResolver,
		events:       make(chan model.DriverEvent, cfg.QueueSize),
		deviceCache:  deviceCache,
		done:         make(chan struct{}),
	}
	go subscriber.run()
	return subscriber, nil
}

// GetAndFlush returns queued driver events and clears the queue.
// It returns an error when the subscriber has stopped.
func (s *DriverEventSubscriber) GetAndFlush() ([]model.DriverEvent, error) {
	events := make([]model.DriverEvent, 0, len(s.events))
	for {
		select {
		case event, ok := <-s.events:
			if !ok {
				return events, errDriverEventSubscriberStopped
			}
			events = append(events, event)
		default:
			return events, nil
		}
	}
}

// Stop terminates the kernel reader and waits for the subscriber to exit.
func (s *DriverEventSubscriber) Stop() {
	s.stopOnce.Do(func() {
		s.reader.Stop()
		<-s.done
	})
}

func (s *DriverEventSubscriber) run() {
	defer func() {
		close(s.events)
		close(s.done)
	}()

	for {
		select {
		case record, ok := <-s.reader.Records():
			if !ok {
				return
			}
			event, err := s.createDriverEvent(record)
			if err != nil && logLimit.ShouldLog() {
				log.Errorf("failed to create driver event: %v", err)
			} else if err == nil {
				s.enqueue(event)
			}
		case err, ok := <-s.reader.Errors():
			if ok {
				log.Errorf("GPU driver event reader stopped: %v", err)
			}
			return
		}
	}
}

func (s *DriverEventSubscriber) createDriverEvent(record kernel.KmsgRecord) (model.DriverEvent, error) {
	var event model.DriverEvent

	pciBusID, ok := parseBusID(record)
	if !ok {
		s.telemetry.parseFailures.Inc("missing_pci_bus_id")
		return event, errors.New("can't find PCI bus ID in message")
	}

	event.Timestamp = s.timeResolver.ResolveMonotonicTimestamp(record.Timestamp * uint64(time.Microsecond))

	var err error
	switch {
	case isNvidiaXidMessage(record):
		err = parseNvidiaXid(record, &event)
	default:
		err = errors.New("message not recognized")
	}

	if err != nil {
		s.telemetry.parseFailures.Inc("parse_failure")
		return event, err
	}

	device, err := s.deviceCache.GetByPCIBusID(pciBusID)
	if err != nil {
		s.telemetry.unresolvedPCI.Inc()
		return event, fmt.Errorf("resolve device UUID for PCI bus ID %s: %w", pciBusID, err)
	}
	event.DeviceUUID = device.GetDeviceInfo().UUID

	return event, nil
}

func (s *DriverEventSubscriber) enqueue(event model.DriverEvent) {
	select {
	case s.events <- event:
	default:
		s.telemetry.dropped.Inc()
	}
}

func isNvidiaXidMessage(record kernel.KmsgRecord) bool {
	return nvidiaXidPrefixPattern.MatchString(record.Message)
}

func parseBusID(record kernel.KmsgRecord) (string, bool) {
	matches := nvidiaPCIBusIDPattern.FindStringSubmatch(record.Message)
	if matches == nil {
		return "", false
	}
	return strings.ToLower(matches[1]) + ".0", true
}

func parseNvidiaXid(record kernel.KmsgRecord, event *model.DriverEvent) error {
	xidCodeMatches := nvidiaXidCodePattern.FindStringSubmatch(record.Message)
	if xidCodeMatches == nil {
		return errors.New("missing Xid code")
	}

	xidCode, err := strconv.ParseUint(xidCodeMatches[1], 10, 64)
	if err != nil {
		return fmt.Errorf("parse Xid code: %w", err)
	}

	event.Type = model.DriverEventTypeNvidiaXid
	event.XidCode = xidCode

	return nil
}
