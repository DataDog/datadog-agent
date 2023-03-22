// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"syscall"
	"unsafe"

	"github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type monitorState = string

const (
	Disabled   monitorState = "Disabled"
	Running    monitorState = "Running"
	NotRunning monitorState = "Not Running"
)

var (
	state        = Disabled
	startupError error
)

// Monitor is responsible for:
// * Creating a raw socket and attaching an eBPF filter to it;
// * Consuming HTTP transaction "events" that are sent from Kernel space;
// * Aggregating and emitting metrics based on the received HTTP transactions;
type Monitor struct {
	httpConsumer    *events.Consumer
	http2Consumer   *events.Consumer
	ebpfProgram     *ebpfProgram
	httpTelemetry   *telemetry
	http2Telemetry  *telemetry
	statkeeper      *httpStatKeeper
	http2Statkeeper *httpStatKeeper
	processMonitor  *monitor.ProcessMonitor

	http2Enabled bool

	// Kafka related
	kafkaEnabled    bool
	kafkaConsumer   *events.Consumer
	kafkaTelemetry  *kafka.Telemetry
	kafkaStatkeeper *kafka.KafkaStatKeeper
	// termination
	closeFilterFn func()
}

// The staticTableEntry represents an entry in the static table that contains an index in the table and a value.
// The value itself contains both the key and the corresponding value in the static table.
// For instance, index 2 in the static table has a value of method: GET, and index 3 has a value of method: POST.
// It is not possible to save the index by the key because we need to distinguish between the values attached to the key.
type staticTableEntry struct {
	Index uint64
	Value StaticTableValue
}

// NewMonitor returns a new Monitor instance
func NewMonitor(c *config.Config, offsets []manager.ConstantEditor, sockFD *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (m *Monitor, err error) {
	defer func() {
		// capture error and wrap it
		if err != nil {
			state = NotRunning
			err = fmt.Errorf("could not instantiate http monitor: %w", err)
			startupError = err
		}
	}()

	if !c.EnableHTTPMonitoring {
		state = Disabled
		return nil, nil
	}

	kversion, err := kernel.HostVersion()
	if err != nil {
		return nil, &ErrNotSupported{fmt.Errorf("couldn't determine current kernel version: %w", err)}
	}

	if kversion < MinimumKernelVersion {
		return nil, &ErrNotSupported{
			fmt.Errorf("http feature not available on pre %s kernels", MinimumKernelVersion.String()),
		}
	}

	mgr, err := newEBPFProgram(c, offsets, sockFD, bpfTelemetry)
	if err != nil {
		return nil, fmt.Errorf("error setting up http ebpf program: %w", err)
	}

	if err := mgr.Init(); err != nil {
		return nil, fmt.Errorf("error initializing http ebpf program: %w", err)
	}

	if c.EnableHTTP2Monitoring {
		err := m.createStaticTable(mgr)
		if err != nil {
			return nil, fmt.Errorf("error creating a static table for http2 monitoring: %w", err)
		}
	}

	filter, _ := mgr.GetProbe(manager.ProbeIdentificationPair{EBPFFuncName: protocolDispatcherSocketFilterFunction, UID: probeUID})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter")
	}

	closeFilterFn, err := filterpkg.HeadlessSocketFilter(c, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling HTTP traffic inspection: %s", err)
	}

	httpTelemetry, err := newTelemetry()
	if err != nil {
		closeFilterFn()
		return nil, err
	}

	statkeeper := newHTTPStatkeeper(c, httpTelemetry)
	processMonitor := monitor.GetProcessMonitor()

	var http2Statkeeper *httpStatKeeper
	var http2Telemetry *telemetry
	if c.EnableHTTP2Monitoring {
		http2Telemetry, err = newTelemetry()
		if err != nil {
			closeFilterFn()
			return nil, err
		}
		// for now the max HTTP2 entries would be taken from the maxHTTPEntries.
		http2Statkeeper = newHTTPStatkeeper(c, http2Telemetry)
	}

	state = Running

	httpMonitor := &Monitor{
		ebpfProgram:     mgr,
		httpTelemetry:   httpTelemetry,
		http2Telemetry:  http2Telemetry,
		closeFilterFn:   closeFilterFn,
		statkeeper:      statkeeper,
		processMonitor:  processMonitor,
		http2Enabled:    c.EnableHTTP2Monitoring,
		http2Statkeeper: http2Statkeeper,
	}

	if c.EnableKafkaMonitoring {
		// Kafka related
		kafkaTelemetry := kafka.NewTelemetry()
		kafkaStatkeeper := kafka.NewKafkaStatkeeper(c, kafkaTelemetry)
		httpMonitor.kafkaEnabled = true
		httpMonitor.kafkaTelemetry = kafkaTelemetry
		httpMonitor.kafkaStatkeeper = kafkaStatkeeper
	}

	return httpMonitor, nil
}

// Start consuming HTTP events
func (m *Monitor) Start() error {
	if m == nil {
		return nil
	}

	var err error

	defer func() {
		if err != nil {
			if errors.Is(err, syscall.ENOMEM) {
				err = fmt.Errorf("could not enable http monitoring: not enough memory to attach http ebpf socket filter. please consider raising the limit via sysctl -w net.core.optmem_max=<LIMIT>")
			}

			if err != nil {
				err = fmt.Errorf("could not enable http monitoring: %s", err)
			}
			startupError = err
		}
	}()

	m.httpConsumer, err = events.NewConsumer(
		"http",
		m.ebpfProgram.Manager.Manager,
		m.processHTTP,
	)
	if err != nil {
		return err
	}
	m.httpConsumer.Start()

	if m.http2Enabled {
		m.http2Consumer, err = events.NewConsumer(
			"http2",
			m.ebpfProgram.Manager.Manager,
			m.processHTTP2,
		)
		if err != nil {
			return err
		}
		m.http2Consumer.Start()
	}

	if m.kafkaEnabled {
		m.kafkaConsumer, err = events.NewConsumer(
			"kafka",
			m.ebpfProgram.Manager.Manager,
			m.kafkaProcess,
		)
		if err != nil {
			return err
		}
		m.kafkaConsumer.Start()
	}

	err = m.ebpfProgram.Start()
	if err != nil {
		return err
	}

	// Need to explicitly save the error in `err` so the defer function could save the startup error.
	err = m.processMonitor.Initialize()
	return err
}

func (m *Monitor) GetUSMStats() map[string]interface{} {
	response := map[string]interface{}{
		"state": state,
	}

	if startupError != nil {
		response["error"] = startupError.Error()
	}

	if m != nil {
		response["last_check"] = m.httpTelemetry.then
	}

	return response
}

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *Monitor) GetHTTPStats() map[Key]*RequestStats {
	if m == nil {
		return nil
	}

	m.httpConsumer.Sync()
	m.httpTelemetry.log()
	return m.statkeeper.GetAndResetAllStats()
}

// GetHTTP2Stats returns a map of HTTP2 stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *Monitor) GetHTTP2Stats() map[Key]*RequestStats {
	if m == nil || m.http2Enabled == false {
		return nil
	}

	m.http2Consumer.Sync()
	m.http2Telemetry.log()
	return m.http2Statkeeper.GetAndResetAllStats()
}

// GetKafkaStats returns a map of Kafka stats
func (m *Monitor) GetKafkaStats() map[kafka.Key]*kafka.RequestStat {
	if m == nil || m.kafkaEnabled == false {
		return nil
	}

	m.kafkaConsumer.Sync()
	m.kafkaTelemetry.Log()
	return m.kafkaStatkeeper.GetAndResetAllStats()
}

// Stop HTTP monitoring
func (m *Monitor) Stop() {
	if m == nil {
		return
	}

	m.processMonitor.Stop()
	m.ebpfProgram.Close()

	m.httpConsumer.Stop()
	if m.http2Enabled {
		m.http2Consumer.Stop()
	}
	if m.kafkaEnabled {
		m.kafkaConsumer.Stop()
	}
	m.closeFilterFn()
}

func (m *Monitor) processHTTP(data []byte) {
	tx := (*ebpfHttpTx)(unsafe.Pointer(&data[0]))
	m.httpTelemetry.count(tx)
	m.statkeeper.Process(tx)
}

func (m *Monitor) processHTTP2(data []byte) {
	tx := (*ebpfHttp2Tx)(unsafe.Pointer(&data[0]))

	m.http2Telemetry.count(tx)
	m.http2Statkeeper.Process(tx)
}

func (m *Monitor) kafkaProcess(data []byte) {
	tx := (*kafka.EbpfKafkaTx)(unsafe.Pointer(&data[0]))
	m.kafkaTelemetry.Count(tx)
	m.kafkaStatkeeper.Process(tx)
}

// DumpMaps dumps the maps associated with the monitor
func (m *Monitor) DumpMaps(maps ...string) (string, error) {
	return m.ebpfProgram.DumpMaps(maps...)
}

// createStaticTable creates a static table for http2 monitor.
func (m *Monitor) createStaticTable(mgr *ebpfProgram) error {
	staticTable, _, _ := mgr.GetMap(probes.StaticTableMap)
	if staticTable == nil {
		return errors.New("http2 static table is null")
	}
	staticTableEntries := []staticTableEntry{
		{
			Index: 2,
			Value: StaticTableValue{
				Key:   MethodKey,
				Value: GetValue,
			},
		},
		{
			Index: 3,
			Value: StaticTableValue{
				Key:   MethodKey,
				Value: PostValue,
			},
		},
		{
			Index: 4,
			Value: StaticTableValue{
				Key:   PathKey,
				Value: EmptyPathValue,
			},
		},
		{
			Index: 5,
			Value: StaticTableValue{
				Key:   PathKey,
				Value: IndexPathValue,
			},
		},
		{
			Index: 8,
			Value: StaticTableValue{
				Key:   StatusKey,
				Value: K200Value,
			},
		},
		{
			Index: 9,
			Value: StaticTableValue{
				Key:   StatusKey,
				Value: K204Value,
			},
		},
		{
			Index: 10,
			Value: StaticTableValue{
				Key:   StatusKey,
				Value: K206Value,
			},
		},
		{
			Index: 11,
			Value: StaticTableValue{
				Key:   StatusKey,
				Value: K304Value,
			},
		},
		{
			Index: 12,
			Value: StaticTableValue{
				Key:   StatusKey,
				Value: K400Value,
			},
		},
		{
			Index: 13,
			Value: StaticTableValue{
				Key:   StatusKey,
				Value: K404Value,
			},
		},
		{
			Index: 14,
			Value: StaticTableValue{
				Key:   StatusKey,
				Value: K500Value,
			},
		},
	}

	for _, entry := range staticTableEntries {
		err := staticTable.Put(unsafe.Pointer(&entry.Index), unsafe.Pointer(&entry.Value))

		if err != nil {
			return err
		}
	}
	return nil
}
