// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf && nvml && test

package gpu

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gputestutil "github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestCreateDriverEvent(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)
	observedAt := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		message string
		xidCode uint64
	}{
		{"NVRM: Xid (PCI:0000:00:1e): 31, pid=1, name=app, channel 0x1. MMU Fault", 31},
		{"NVRM: Xid (PCI:0000:00:1e): 13, Graphics Exception: channel 0x1", 13},
		{"NVRM: Xid (PCI:0000:00:1e): 120, GSP task exception", 120},
		{"NVRM: Xid (PCI:0000:00:1e): 154, GPU recovery action changed", 154},
		{"nvrm:  xid ( pci:0000:00:1e.0 ) : 43, channel 0x1", 43},
	} {
		record := kernel.KmsgRecord{Timestamp: 1234, ObservedAt: observedAt, Message: tc.message}

		event, err := subscriber.createDriverEvent(record)

		require.NoError(t, err, tc.message)
		require.Equal(t, gputestutil.DefaultGpuUUID, event.DeviceUUID, tc.message)
		require.Equal(t, observedAt, event.Timestamp, tc.message)
		require.Equal(t, model.DriverEventTypeNvidiaXid, event.Type, tc.message)
		require.Equal(t, tc.xidCode, event.NvidiaXid.XidCode, tc.message)
		require.Equal(t, tc.message, event.NvidiaXid.Message, tc.message)
	}
}

func TestParseNvidiaXidDetails(t *testing.T) {
	for _, tc := range []struct {
		name     string
		message  string
		expected model.NvidiaXid
	}{
		{
			name:    "MMU PDE fault with process",
			message: "NVRM: Xid (PCI:0000:35:00): 31, pid=3634001, name=vectorAdd, channel 0x01000020, intr 00000000. MMU Fault: ENGINE CE2 HUBCLIENT_CE0 faulted @ 0x792c_58000000. Fault is of type FAULT_PDE ACCESS_TYPE_VIRT_WRITE",
			expected: model.NvidiaXid{
				XidCode:     31,
				Message:     "NVRM: Xid (PCI:0000:35:00): 31, pid=3634001, name=vectorAdd, channel 0x01000020, intr 00000000. MMU Fault: ENGINE CE2 HUBCLIENT_CE0 faulted @ 0x792c_58000000. Fault is of type FAULT_PDE ACCESS_TYPE_VIRT_WRITE",
				ProcessID:   uint64Pointer(3634001),
				ProcessName: "vectorAdd",
				Channel:     "0x01000020",
				MMUFault: &model.NvidiaXidMMUFault{
					Interrupt:    "0x00000000",
					Engine:       "CE2",
					EngineClient: "HUBCLIENT_CE0",
					FaultAddress: "0x792c58000000",
					FaultType:    "FAULT_PDE",
					AccessType:   "ACCESS_TYPE_VIRT_WRITE",
				},
			},
		},
		{
			name:    "MMU PTE fault with unknown process",
			message: "NVRM: Xid (PCI:0000:00:1e): 31, pid=8, name=<unknown>, channel 0x00000004. MMU Fault: ENGINE CE0 HUBCLIENT_HOST faulted @ 0x0000000000001000. FAULT_PTE ACCESS_TYPE_ATOMIC",
			expected: model.NvidiaXid{
				XidCode:     31,
				Message:     "NVRM: Xid (PCI:0000:00:1e): 31, pid=8, name=<unknown>, channel 0x00000004. MMU Fault: ENGINE CE0 HUBCLIENT_HOST faulted @ 0x0000000000001000. FAULT_PTE ACCESS_TYPE_ATOMIC",
				ProcessID:   uint64Pointer(8),
				ProcessName: "<unknown>",
				Channel:     "0x00000004",
				MMUFault: &model.NvidiaXidMMUFault{
					Engine:       "CE0",
					EngineClient: "HUBCLIENT_HOST",
					FaultAddress: "0x0000000000001000",
					FaultType:    "FAULT_PTE",
					AccessType:   "ACCESS_TYPE_ATOMIC",
				},
			},
		},
		{
			// Xid 43 has no detail parser, so the channel has to come from the preamble.
			name:    "channel is captured for a code with no detail parser",
			message: "NVRM: Xid (PCI:0000:00:1b): 43, pid=3885519, name=gpu-burner, channel 0x00000010",
			expected: model.NvidiaXid{
				XidCode:     43,
				Message:     "NVRM: Xid (PCI:0000:00:1b): 43, pid=3885519, name=gpu-burner, channel 0x00000010",
				ProcessID:   uint64Pointer(3885519),
				ProcessName: "gpu-burner",
				Channel:     "0x00000010",
			},
		},
		{
			name:    "NVLink with process and status",
			message: "NVRM: Xid (PCI:0000:00:1e): 145, pid=4, name=<unknown>, RLW_SRC_TRACK Fatal XC0 i1 Link 00 (0x00000001 0x00000002)",
			expected: model.NvidiaXid{
				XidCode:     145,
				Message:     "NVRM: Xid (PCI:0000:00:1e): 145, pid=4, name=<unknown>, RLW_SRC_TRACK Fatal XC0 i1 Link 00 (0x00000001 0x00000002)",
				ProcessID:   uint64Pointer(4),
				ProcessName: "<unknown>",
				NVLinkFault: &model.NvidiaXidNVLinkFault{
					Subcode:          "RLW_SRC_TRACK",
					Fatal:            true,
					CrossContainment: false,
					Injected:         true,
					LinkID:           uint64Pointer(0),
					IntrInfo:         "0x00000001",
					ErrorStatus:      "0x00000002",
				},
			},
		},
		{
			name:    "NVLink without process",
			message: "NVRM: Xid (PCI:0000:00:1e): 149, NETIR_LINK_DOWN Nonfatal XC1 i2 Link 17 (0xdeadbeef)",
			expected: model.NvidiaXid{
				XidCode: 149,
				Message: "NVRM: Xid (PCI:0000:00:1e): 149, NETIR_LINK_DOWN Nonfatal XC1 i2 Link 17 (0xdeadbeef)",
				NVLinkFault: &model.NvidiaXidNVLinkFault{
					Subcode:          "NETIR_LINK_DOWN",
					Fatal:            false,
					CrossContainment: true,
					Injected:         true,
					LinkID:           uint64Pointer(17),
					IntrInfo:         "0xdeadbeef",
				},
			},
		},
		{
			name:    "NVLink pre R575 link event",
			message: "NVRM: Xid (PCI:0000:00:1e): 149, NETIR_LINK_EVT Fatal XC 1 i 2 Link 01 [0x0000000a]",
			expected: model.NvidiaXid{
				XidCode: 149,
				Message: "NVRM: Xid (PCI:0000:00:1e): 149, NETIR_LINK_EVT Fatal XC 1 i 2 Link 01 [0x0000000a]",
				NVLinkFault: &model.NvidiaXidNVLinkFault{
					Subcode:          "NETIR_LINK_EVT",
					Fatal:            true,
					CrossContainment: true,
					Injected:         true,
					LinkID:           uint64Pointer(1),
					IntrInfo:         "0x0000000a",
				},
			},
		},
		{
			name:    "NVLink observed nonfatal fault keeps decode words in printed order",
			message: "NVRM: Xid (PCI:0000:00:1e): 146, TLW_RX_PIPE1 Nonfatal XC0 i0 Link 03 (0x00000010 0x00000020 0x00000030 0x00000040)",
			expected: model.NvidiaXid{
				XidCode: 146,
				Message: "NVRM: Xid (PCI:0000:00:1e): 146, TLW_RX_PIPE1 Nonfatal XC0 i0 Link 03 (0x00000010 0x00000020 0x00000030 0x00000040)",
				NVLinkFault: &model.NvidiaXidNVLinkFault{
					Subcode:          "TLW_RX_PIPE1",
					Fatal:            false,
					CrossContainment: false,
					Injected:         false,
					LinkID:           uint64Pointer(3),
					IntrInfo:         "0x00000010",
					ErrorStatus:      "0x00000020",
					ErrorDebugData:   []string{"0x00000030", "0x00000040"},
				},
			},
		},
		{
			name:    "NVLink zero-padded flags are read numerically",
			message: "NVRM: Xid (PCI:0000:00:1e): 147, TREX_ERR Nonfatal XC 00 i 00 Link 02 (0x00000005)",
			expected: model.NvidiaXid{
				XidCode: 147,
				Message: "NVRM: Xid (PCI:0000:00:1e): 147, TREX_ERR Nonfatal XC 00 i 00 Link 02 (0x00000005)",
				NVLinkFault: &model.NvidiaXidNVLinkFault{
					Subcode:          "TREX_ERR",
					Fatal:            false,
					CrossContainment: false,
					Injected:         false,
					LinkID:           uint64Pointer(2),
					IntrInfo:         "0x00000005",
				},
			},
		},
		{
			name:    "NVLink fault without a decode group",
			message: "NVRM: Xid (PCI:0000:00:1e): 148, NVLPW_CTRL Fatal XC0 i0 Link 05",
			expected: model.NvidiaXid{
				XidCode: 148,
				Message: "NVRM: Xid (PCI:0000:00:1e): 148, NVLPW_CTRL Fatal XC0 i0 Link 05",
				NVLinkFault: &model.NvidiaXidNVLinkFault{
					Subcode: "NVLPW_CTRL",
					Fatal:   true,
					LinkID:  uint64Pointer(5),
				},
			},
		},
		{
			name:    "memory ECC and repair chain",
			message: "NVRM: Xid (PCI:0000:00:1e): 160, Marking Channel 3 in FBPA 2 for repair; Node Reboot Required",
			expected: model.NvidiaXid{
				XidCode: 160,
				Message: "NVRM: Xid (PCI:0000:00:1e): 160, Marking Channel 3 in FBPA 2 for repair; Node Reboot Required",
				MemoryFault: &model.NvidiaXidMemoryFault{
					RepairedTarget:      "channel",
					RepairedTargetIndex: uint64Pointer(3),
					FBPA:                uint64Pointer(2),
					NodeRebootRequired:  true,
				},
			},
		},
		{
			name:    "memory address and location",
			message: "NVRM: Xid (PCI:0000:00:1e): 48, Double Bit ECC Error at physAddr 0x0000_0000_0123_4567, partition 1, subpartition 2",
			expected: model.NvidiaXid{
				XidCode: 48,
				Message: "NVRM: Xid (PCI:0000:00:1e): 48, Double Bit ECC Error at physAddr 0x0000_0000_0123_4567, partition 1, subpartition 2",
				MemoryFault: &model.NvidiaXidMemoryFault{
					PhysicalAddress: "0x0000000001234567",
					Partition:       uint64Pointer(1),
					Subpartition:    uint64Pointer(2),
				},
			},
		},
		{
			name:    "row remapper address",
			message: "NVRM: Xid (PCI:0000:00:1e): 63, Row Remapper failed at row address 0x000000000000abcd site FBPA0",
			expected: model.NvidiaXid{
				XidCode: 63,
				Message: "NVRM: Xid (PCI:0000:00:1e): 63, Row Remapper failed at row address 0x000000000000abcd site FBPA0",
				MemoryFault: &model.NvidiaXidMemoryFault{
					RowAddress:      "0x000000000000abcd",
					RowRemapperSite: "FBPA0",
				},
			},
		},
		{
			name:    "contained SRAM ECC",
			message: "NVRM: Xid (PCI:0000:00:1e): 94, Contained SRAM ECC error",
			expected: model.NvidiaXid{
				XidCode: 94,
				Message: "NVRM: Xid (PCI:0000:00:1e): 94, Contained SRAM ECC error",
				MemoryFault: &model.NvidiaXidMemoryFault{
					Location: "SRAM",
				},
			},
		},
		{
			name:    "uncontained DRAM ECC",
			message: "NVRM: Xid (PCI:0000:00:1e): 95, Uncontained DRAM ECC error",
			expected: model.NvidiaXid{
				XidCode: 95,
				Message: "NVRM: Xid (PCI:0000:00:1e): 95, Uncontained DRAM ECC error",
				MemoryFault: &model.NvidiaXidMemoryFault{
					Location: "DRAM",
				},
			},
		},
		{
			name:    "DRAM annotation",
			message: "NVRM: Xid (PCI:0000:00:1e): 171, Double Bit ECC Error in FBPA 4 subpartition 1",
			expected: model.NvidiaXid{
				XidCode: 171,
				Message: "NVRM: Xid (PCI:0000:00:1e): 171, Double Bit ECC Error in FBPA 4 subpartition 1",
				MemoryFault: &model.NvidiaXidMemoryFault{
					FBPA:         uint64Pointer(4),
					Subpartition: uint64Pointer(1),
				},
			},
		},
		{
			name:    "recovery action",
			message: "NVRM: Xid (PCI:0000:00:1e): 154, GPU recovery action changed from 0x0 (None) to 0x1 (Drain and Reset)",
			expected: model.NvidiaXid{
				XidCode: 154,
				Message: "NVRM: Xid (PCI:0000:00:1e): 154, GPU recovery action changed from 0x0 (None) to 0x1 (Drain and Reset)",
				RecoveryAction: &model.NvidiaXidRecoveryAction{
					PreviousCode:  uint64Pointer(0),
					PreviousLabel: "None",
					CurrentCode:   uint64Pointer(1),
					CurrentLabel:  "Drain and Reset",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var event model.DriverEvent
			_, err := parseNvidiaXid(kernel.KmsgRecord{Message: tc.message}, &event)
			require.NoError(t, err)
			require.Equal(t, model.DriverEvent{Type: model.DriverEventTypeNvidiaXid, NvidiaXid: &tc.expected}, event)
		})
	}
}

func TestParseNvidiaXidPreservesGenericEventsWhenDetailsAreMissing(t *testing.T) {
	for _, message := range []string{
		"NVRM: Xid (PCI:0000:00:1e): 145, driver-specific detail",
		"NVRM: Xid (PCI:0000:00:1e): 999, pid=5, name=<unknown>, future driver detail",
	} {
		var event model.DriverEvent

		_, err := parseNvidiaXid(kernel.KmsgRecord{Message: message}, &event)
		require.NoError(t, err)
		require.Equal(t, model.DriverEventTypeNvidiaXid, event.Type)
		require.Equal(t, message, event.NvidiaXid.Message)
		require.Nil(t, event.NvidiaXid.MMUFault)
		require.Nil(t, event.NvidiaXid.NVLinkFault)
		require.Nil(t, event.NvidiaXid.MemoryFault)
		require.Nil(t, event.NvidiaXid.RecoveryAction)
	}
}

func TestCreateDriverEventCountsMalformedOptionalDetails(t *testing.T) {
	subscriber, telemetryMock := newTestDriverEventSubscriber(t)

	event, err := subscriber.createDriverEvent(kernel.KmsgRecord{
		Message: "NVRM: Xid (PCI:0000:00:1e): 145, driver-specific detail",
	})

	require.NoError(t, err)
	require.Equal(t, uint64(145), event.NvidiaXid.XidCode)
	require.Nil(t, event.NvidiaXid.NVLinkFault)
	enrichmentMetrics, err := telemetryMock.GetCountMetric("gpu__driver_events", "enrichment_failures")
	require.NoError(t, err)
	require.Len(t, enrichmentMetrics, 1)
	require.Equal(t, float64(1), enrichmentMetrics[0].Value())
}

func TestParseNvidiaXidBoundsRawMessage(t *testing.T) {
	message := "NVRM: Xid (PCI:0000:00:1e): 13, " + strings.Repeat("x", maxDriverEventMessageLength)
	var event model.DriverEvent

	_, err := parseNvidiaXid(kernel.KmsgRecord{Message: message}, &event)
	require.NoError(t, err)
	require.Len(t, event.NvidiaXid.Message, maxDriverEventMessageLength)
}

func uint64Pointer(value uint64) *uint64 {
	return &value
}

func TestCreateDriverEventKeepsEventWhenDeviceIsUnresolved(t *testing.T) {
	subscriber, telemetryMock := newTestDriverEventSubscriber(t)

	// A GPU that has fallen off the PCIe bus cannot be resolved through NVML, so the event
	// must survive with the PCI bus ID as its only device identifier.
	event, err := subscriber.createDriverEvent(kernel.KmsgRecord{Message: "NVRM: Xid (PCI:0000:97:00): 79, GPU has fallen off the bus"})

	require.NoError(t, err)
	require.Empty(t, event.DeviceUUID)
	require.Equal(t, "0000:97:00.0", event.PCIBusID)
	require.Equal(t, uint64(79), event.NvidiaXid.XidCode)

	unresolvedMetrics, err := telemetryMock.GetCountMetric("gpu__driver_events", "unresolved_pci")
	require.NoError(t, err)
	require.Len(t, unresolvedMetrics, 1)
	require.Equal(t, float64(1), unresolvedMetrics[0].Value())
}

func TestCreateDriverEventRecordsPCIBusIDOnResolvedDevices(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)

	event, err := subscriber.createDriverEvent(kernel.KmsgRecord{Message: "NVRM: Xid (PCI:0000:00:1e): 31"})

	require.NoError(t, err)
	require.Equal(t, gputestutil.DefaultGpuUUID, event.DeviceUUID)
	require.Equal(t, "0000:00:1e.0", event.PCIBusID)
}

func TestCreateDriverEventRejectsMalformedMessages(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)

	for _, message := range []string{
		"NVRM: Xid (PCI:0000:65:00): invalid",
		"NVRM: Xid (PCI:0000:65:00)",
		"NVRM: Xid: 31",
	} {
		_, err := subscriber.createDriverEvent(kernel.KmsgRecord{Message: message})
		require.Error(t, err, message)
	}
}

func TestDriverEventQueueDropsWhenFull(t *testing.T) {
	subscriber, telemetryMock := newTestDriverEventSubscriber(t)
	first := model.DriverEvent{DeviceUUID: "GPU-first"}
	second := model.DriverEvent{DeviceUUID: "GPU-second"}

	subscriber.enqueue(first)
	subscriber.enqueue(second)

	events, err := subscriber.GetAndFlush()
	require.NoError(t, err)
	require.Equal(t, []model.DriverEvent{first}, events)

	droppedMetrics, err := telemetryMock.GetCountMetric("gpu__driver_events", "dropped")
	require.NoError(t, err)
	require.Len(t, droppedMetrics, 1)
	require.Equal(t, float64(1), droppedMetrics[0].Value())

	events, err = subscriber.GetAndFlush()
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestDriverEventSubscriberStop(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)
	reader := newFakeDriverEventReader()
	subscriber.reader = reader
	subscriber.records = reader.records
	subscriber.done = make(chan struct{})
	go subscriber.run()
	queuedEvent := model.DriverEvent{DeviceUUID: "GPU-queued"}
	subscriber.enqueue(queuedEvent)

	subscriber.Stop()
	subscriber.Stop()

	events, err := subscriber.GetAndFlush()
	require.Equal(t, []model.DriverEvent{queuedEvent}, events)
	require.NoError(t, err)

	events, err = subscriber.GetAndFlush()
	require.Empty(t, events)
	require.ErrorIs(t, err, errDriverEventSubscriberStopped)
}

func TestDriverEventSubscriberStopsAfterReaderError(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)
	reader := newFakeDriverEventReader()
	subscriber.reader = reader
	subscriber.records = reader.records
	subscriber.done = make(chan struct{})
	go subscriber.run()

	reader.errors <- errors.New("read failed")
	<-subscriber.done

	events, err := subscriber.GetAndFlush()
	require.Empty(t, events)
	require.ErrorIs(t, err, errDriverEventSubscriberStopped)
}

func newTestDriverEventSubscriber(t *testing.T) (*DriverEventSubscriber, telemetry.Mock) {
	ddnvml.WithMockNVML(t, gputestutil.NewMockNVML(gputestutil.WithDeviceCount(1)))

	deviceCache := ddnvml.NewDeviceCache()
	require.NoError(t, deviceCache.Refresh())

	telemetryMock := gputestutil.GetTelemetryMock(t)
	telemetry := &driverEventTelemetry{}
	telemetry.init(telemetryMock)

	return &DriverEventSubscriber{
		telemetry:   telemetry,
		events:      make(chan model.DriverEvent, 1),
		deviceCache: deviceCache,
	}, telemetryMock
}

type fakeDriverEventReader struct {
	records  chan kernel.KmsgRecord
	errors   chan error
	stopOnce sync.Once
}

func newFakeDriverEventReader() *fakeDriverEventReader {
	return &fakeDriverEventReader{
		records: make(chan kernel.KmsgRecord),
		errors:  make(chan error, 1),
	}
}

func (r *fakeDriverEventReader) Records() <-chan kernel.KmsgRecord {
	return r.records
}

func (r *fakeDriverEventReader) Errors() <-chan error {
	return r.errors
}

func (r *fakeDriverEventReader) Stop() {
	r.stopOnce.Do(func() {
		close(r.errors)
		close(r.records)
	})
}
