// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
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

	// knownProtocols maps individual protocol types, to their specification,
	// for the Monitor to use during its initialisation.
	knownProtocols = map[protocols.ProtocolType]protocols.ProtocolSpec{
		protocols.HTTP: http.Spec,
	}
)

var errNoProtocols = errors.New("no protocol monitors were initialised")

// Monitor is responsible for:
// * Creating a raw socket and attaching an eBPF filter to it;
// * Consuming HTTP transaction "events" that are sent from Kernel space;
// * Aggregating and emitting metrics based on the received HTTP transactions;
type Monitor struct {
	enabledProtocols map[protocols.ProtocolType]protocols.Protocol

	http2Consumer   *events.Consumer
	ebpfProgram     *ebpfProgram
	http2Telemetry  *http.Telemetry
	http2Statkeeper *http.StatKeeper
	processMonitor  *monitor.ProcessMonitor

	http2Enabled   bool
	httpTLSEnabled bool

	// Kafka related
	kafkaEnabled    bool
	kafkaConsumer   *events.Consumer
	kafkaTelemetry  *kafka.Telemetry
	kafkaStatkeeper *kafka.KafkaStatKeeper
	// termination
	closeFilterFn func()

	lastUpdateTime *atomic.Int64
}

// The staticTableEntry represents an entry in the static table that contains an index in the table and a value.
// The value itself contains both the key and the corresponding value in the static table.
// For instance, index 2 in the static table has a value of method: GET, and index 3 has a value of method: POST.
// It is not possible to save the index by the key because we need to distinguish between the values attached to the key.
type staticTableEntry struct {
	Index uint64
	Value http2.StaticTableValue
}

// NewMonitor returns a new Monitor instance
func NewMonitor(c *config.Config, connectionProtocolMap, sockFD *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) (m *Monitor, err error) {
	defer func() {
		// capture error and wrap it
		if err != nil {
			state = NotRunning
			err = fmt.Errorf("could not initialize USM: %w", err)
			startupError = err
		}
	}()

	mgr, err := newEBPFProgram(c, connectionProtocolMap, sockFD, bpfTelemetry)
	if err != nil {
		return nil, fmt.Errorf("error setting up ebpf program: %w", err)
	}

	enabledProtocols, disabledProtocols, err := initProtocols(c, mgr)
	if err != nil {
		return nil, err
	}
	if len(enabledProtocols) == 0 {
		state = Disabled
		log.Debug("not enabling USM as no protocols monitoring were enabled.")
		return nil, nil
	}

	mgr.enabledProtocols = enabledProtocols
	mgr.disabledProtocols = disabledProtocols

	if err := mgr.Init(); err != nil {
		return nil, fmt.Errorf("error initializing ebpf program: %w", err)
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
		return nil, fmt.Errorf("error enabling traffic inspection: %s", err)
	}

	processMonitor := monitor.GetProcessMonitor()

	var http2Statkeeper *http.StatKeeper
	var http2Telemetry *http.Telemetry
	if c.EnableHTTP2Monitoring {
		http2Telemetry = http.NewTelemetry()

		// for now the max HTTP2 entries would be taken from the maxHTTPEntries.
		http2Statkeeper = http.NewStatkeeper(c, http2Telemetry)
	}

	state = Running

	usmMonitor := &Monitor{
		enabledProtocols: enabledProtocols,
		ebpfProgram:      mgr,
		http2Telemetry:   http2Telemetry,
		closeFilterFn:    closeFilterFn,
		processMonitor:   processMonitor,
		http2Enabled:     c.EnableHTTP2Monitoring,
		http2Statkeeper:  http2Statkeeper,
		httpTLSEnabled:   c.EnableHTTPSMonitoring,
	}

	if c.EnableKafkaMonitoring {
		// Kafka related
		kafkaTelemetry := kafka.NewTelemetry()
		kafkaStatkeeper := kafka.NewKafkaStatkeeper(c, kafkaTelemetry)
		usmMonitor.kafkaEnabled = true
		usmMonitor.kafkaTelemetry = kafkaTelemetry
		usmMonitor.kafkaStatkeeper = kafkaStatkeeper
	}

	usmMonitor.lastUpdateTime = atomic.NewInt64(time.Now().Unix())

	return usmMonitor, nil
}

// Start USM monitor.
func (m *Monitor) Start() error {
	if m == nil {
		return nil
	}

	var (
		err error

		// This value is here so that both the new way of handling protocols and
		// the old one (used here by kafka & http2 monitoring) can coexist in
		// this function. It SHOULD be removed once every protocol has been
		// refactored to use the new way.
		enabledCount int = 0
	)

	defer func() {
		if err != nil {
			if errors.Is(err, syscall.ENOMEM) {
				err = fmt.Errorf("could not enable usm monitoring: not enough memory to attach http ebpf socket filter. please consider raising the limit via sysctl -w net.core.optmem_max=<LIMIT>")
			}

			// Cleanup every remaining protocols
			for _, protocol := range m.enabledProtocols {
				protocol.Stop(m.ebpfProgram.Manager.Manager)
			}

			if err != nil {
				err = fmt.Errorf("could not enable USM: %s", err)
			}
			startupError = err
		}
	}()

	for protocolType, protocol := range m.enabledProtocols {
		startErr := protocol.PreStart(m.ebpfProgram.Manager.Manager)
		if startErr != nil {
			delete(m.enabledProtocols, protocolType)
			log.Errorf("could not complete pre-start phase of %s monitoring: %s", protocolType, startErr)
			continue
		}

		enabledCount++
	}

	if m.http2Enabled {
		m.http2Consumer, err = events.NewConsumer(
			"http2",
			m.ebpfProgram.Manager.Manager,
			m.processHTTP2,
		)
		if err != nil {
			log.Errorf("could not enable http2 monitoring: %s", err)
		} else {
			m.http2Consumer.Start()
			enabledCount++
		}
	}

	if m.kafkaEnabled {
		m.kafkaConsumer, err = events.NewConsumer(
			"kafka",
			m.ebpfProgram.Manager.Manager,
			m.kafkaProcess,
		)
		if err != nil {
			log.Errorf("could not enable kafka monitoring: %s", err)
		} else {
			m.kafkaConsumer.Start()
			enabledCount++
		}
	}

	// No protocols could be enabled, abort.
	if enabledCount == 0 {
		return errNoProtocols
	}

	err = m.ebpfProgram.Start()
	if err != nil {
		return err
	}

	for protocolType, protocol := range m.enabledProtocols {
		startErr := protocol.PostStart(m.ebpfProgram.Manager.Manager)
		if startErr != nil {
			// Cleanup the protocol. Note that at this point we can't unload the
			// ebpf programs of a specific protocol without shutting down the
			// entire manager.
			enabledCount--
			protocol.Stop(m.ebpfProgram.Manager.Manager)
			delete(m.enabledProtocols, protocolType)

			// Log and reset the error value
			log.Errorf("could not complete post-start phase of %s monitoring: %s", protocolType, startErr)
		}
	}

	// We check again if there are protocols that could be enabled, and abort if
	// it is not the case.
	if enabledCount == 0 {
		err = m.ebpfProgram.Close()
		if err != nil {
			log.Errorf("error during USM shutdown: %s", err)
		}

		return errNoProtocols
	}

	// Need to explicitly save the error in `err` so the defer function could save the startup error.
	if m.httpTLSEnabled {
		err = m.processMonitor.Initialize()
	}

	for protocolName := range m.enabledProtocols {
		log.Infof("enabled USM protocol: %s", protocolName)
	}

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
		response["last_check"] = m.lastUpdateTime
	}
	return response
}

func (m *Monitor) GetProtocolStats() map[protocols.ProtocolType]interface{} {
	if m == nil {
		return nil
	}

	defer func() {
		// Update update time
		now := time.Now().Unix()
		m.lastUpdateTime.Swap(now)
		telemetry.ReportPrometheus()
	}()

	ret := make(map[protocols.ProtocolType]interface{})

	for _, protocol := range m.enabledProtocols {
		ps := protocol.GetStats()
		ret[ps.Type] = ps.Stats
	}

	return ret
}

// GetHTTP2Stats returns a map of HTTP2 stats stored in the following format:
// [source, dest tuple, request path] -> RequestStats object
func (m *Monitor) GetHTTP2Stats() map[http.Key]*http.RequestStats {
	if m == nil || m.http2Enabled == false {
		return nil
	}

	m.http2Consumer.Sync()
	m.http2Telemetry.Log()
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

	for _, protocol := range m.enabledProtocols {
		protocol.Stop(m.ebpfProgram.Manager.Manager)
	}

	m.ebpfProgram.Close()

	if m.http2Enabled {
		m.http2Consumer.Stop()
	}
	if m.kafkaEnabled {
		m.kafkaConsumer.Stop()
	}
	if m.http2Statkeeper != nil {
		m.http2Statkeeper.Close()
	}
	m.closeFilterFn()
}

func (m *Monitor) processHTTP2(data []byte) {
	tx := (*http2.EbpfTx)(unsafe.Pointer(&data[0]))

	m.http2Telemetry.Count(tx)
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
			Value: http2.StaticTableValue{
				Key:   http2.MethodKey,
				Value: http2.GetValue,
			},
		},
		{
			Index: 3,
			Value: http2.StaticTableValue{
				Key:   http2.MethodKey,
				Value: http2.PostValue,
			},
		},
		{
			Index: 4,
			Value: http2.StaticTableValue{
				Key:   http2.PathKey,
				Value: http2.EmptyPathValue,
			},
		},
		{
			Index: 5,
			Value: http2.StaticTableValue{
				Key:   http2.PathKey,
				Value: http2.IndexPathValue,
			},
		},
		{
			Index: 8,
			Value: http2.StaticTableValue{
				Key:   http2.StatusKey,
				Value: http2.K200Value,
			},
		},
		{
			Index: 9,
			Value: http2.StaticTableValue{
				Key:   http2.StatusKey,
				Value: http2.K204Value,
			},
		},
		{
			Index: 10,
			Value: http2.StaticTableValue{
				Key:   http2.StatusKey,
				Value: http2.K206Value,
			},
		},
		{
			Index: 11,
			Value: http2.StaticTableValue{
				Key:   http2.StatusKey,
				Value: http2.K304Value,
			},
		},
		{
			Index: 12,
			Value: http2.StaticTableValue{
				Key:   http2.StatusKey,
				Value: http2.K400Value,
			},
		},
		{
			Index: 13,
			Value: http2.StaticTableValue{
				Key:   http2.StatusKey,
				Value: http2.K404Value,
			},
		},
		{
			Index: 14,
			Value: http2.StaticTableValue{
				Key:   http2.StatusKey,
				Value: http2.K500Value,
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

// initProtocols takes the network configuration `c` and uses it to initialise
// the enabled protocols' monitoring, and configures the ebpf-manager `mgr`
// accordingly.
//
// For each enabled protocols, a protocol-specific instance of the Protocol
// interface is initialised, and the required maps and tail calls routers are setup
// in the manager.
//
// If a protocol is not enabled, its tail calls are instead added to the list of
// excluded functions for them to be patched out by ebpf-manager on startup.
//
// It returns:
// - a slice containing instances of the Protocol interface for each enabled protocol support
// - a slice containing pointers to the protocol specs of disabled protocols.
// - an error value, which is non-nil if an error occurred while initialising a protocol
func initProtocols(c *config.Config, mgr *ebpfProgram) (map[protocols.ProtocolType]protocols.Protocol, []*protocols.ProtocolSpec, error) {
	enabledProtocols := make(map[protocols.ProtocolType]protocols.Protocol)
	disabledProtocols := make([]*protocols.ProtocolSpec, 0)

	for proto, spec := range knownProtocols {
		protocol, err := spec.Factory(c)
		if err != nil {
			return nil, nil, &errNotSupported{err}
		}

		if protocol != nil {
			// Configure the manager
			mgr.Maps = append(mgr.Maps, spec.Maps...)
			mgr.tailCallRouter = append(mgr.tailCallRouter, spec.TailCalls...)

			enabledProtocols[proto] = protocol

			log.Infof("%v monitoring enabled", proto.String())
		} else {
			disabledProtocols = append(disabledProtocols, &spec)
		}
	}

	return enabledProtocols, disabledProtocols, nil
}
