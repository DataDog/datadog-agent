// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
)

type xidMergerTestStep struct {
	queryTime             time.Time
	nvmlEvents            []observedDeviceEvent
	driverEvents          []model.DriverEvent
	driverEventsEnabled   bool
	expectedLatest        []xidEvent
	expectedPendingNVML   []xidEvent
	expectedPendingDriver []xidEvent
}

func TestXIDEventMerger(t *testing.T) {
	timestamp := time.Unix(100, 0)

	tests := []struct {
		name  string
		steps []xidMergerTestStep
	}{
		{
			name: "matches both sources and retains fields",
			steps: []xidMergerTestStep{{
				queryTime: timestamp,
				nvmlEvents: []observedDeviceEvent{
					newObservedXIDEvent("GPU-1", 31, timestamp, 4, 7),
				},
				driverEvents: []model.DriverEvent{
					newDriverXIDEvent("GPU-1", 31, timestamp, "kernel message"),
				},
				driverEventsEnabled: true,
				expectedLatest: []xidEvent{
					newMergedXIDEvent(
						newObservedXIDEvent("GPU-1", 31, timestamp, 4, 7),
						newDriverXIDEvent("GPU-1", 31, timestamp, "kernel message"),
					),
				},
			}},
		},
		{
			name: "matches at positive window boundary",
			steps: []xidMergerTestStep{{
				queryTime: timestamp.Add(time.Second),
				nvmlEvents: []observedDeviceEvent{
					newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
				},
				driverEvents: []model.DriverEvent{
					newDriverXIDEvent("GPU-1", 31, timestamp.Add(xidEventMergeWindow), "positive"),
				},
				driverEventsEnabled: true,
				expectedLatest: []xidEvent{
					newMergedXIDEvent(
						newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
						newDriverXIDEvent("GPU-1", 31, timestamp.Add(xidEventMergeWindow), "positive"),
					),
				},
			}},
		},
		{
			name: "matches at negative window boundary",
			steps: []xidMergerTestStep{{
				queryTime: timestamp.Add(time.Second),
				nvmlEvents: []observedDeviceEvent{
					newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
				},
				driverEvents: []model.DriverEvent{
					newDriverXIDEvent("GPU-1", 31, timestamp.Add(-xidEventMergeWindow), "negative"),
				},
				driverEventsEnabled: true,
				expectedLatest: []xidEvent{
					newMergedXIDEvent(
						newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
						newDriverXIDEvent("GPU-1", 31, timestamp.Add(-xidEventMergeWindow), "negative"),
					),
				},
			}},
		},
		{
			name: "does not match outside window",
			steps: []xidMergerTestStep{{
				queryTime: timestamp.Add(time.Second),
				nvmlEvents: []observedDeviceEvent{
					newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
				},
				driverEvents: []model.DriverEvent{
					newDriverXIDEvent("GPU-1", 31, timestamp.Add(xidEventMergeWindow+time.Nanosecond), "outside"),
				},
				driverEventsEnabled: true,
				expectedLatest: []xidEvent{
					newNVMLXIDEvent(newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2)),
					newDriverOnlyXIDEvent(newDriverXIDEvent("GPU-1", 31, timestamp.Add(xidEventMergeWindow+time.Nanosecond), "outside")),
				},
			}},
		},
		{
			name: "requires the same code",
			steps: []xidMergerTestStep{{
				queryTime: timestamp.Add(time.Second),
				nvmlEvents: []observedDeviceEvent{
					newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
					newObservedXIDEvent("GPU-1", 43, timestamp, 5, 6),
				},
				driverEvents: []model.DriverEvent{
					newDriverXIDEvent("GPU-1", 12, timestamp, "other code"),
				},
				driverEventsEnabled: true,
				expectedLatest: []xidEvent{
					newDriverOnlyXIDEvent(newDriverXIDEvent("GPU-1", 12, timestamp, "other code")),
					newNVMLXIDEvent(newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2)),
					newNVMLXIDEvent(newObservedXIDEvent("GPU-1", 43, timestamp, 5, 6)),
				},
			}},
		},
		{
			name: "matches repeated events one to one",
			steps: []xidMergerTestStep{{
				queryTime: timestamp.Add(time.Second),
				nvmlEvents: []observedDeviceEvent{
					newObservedXIDEvent("GPU-1", 31, timestamp, 1, 1),
					newObservedXIDEvent("GPU-1", 31, timestamp.Add(8*time.Millisecond), 2, 2),
				},
				driverEvents: []model.DriverEvent{
					newDriverXIDEvent("GPU-1", 31, timestamp.Add(7*time.Millisecond), "second"),
					newDriverXIDEvent("GPU-1", 31, timestamp.Add(time.Millisecond), "first"),
				},
				driverEventsEnabled: true,
				expectedLatest: []xidEvent{
					newMergedXIDEvent(
						newObservedXIDEvent("GPU-1", 31, timestamp, 1, 1),
						newDriverXIDEvent("GPU-1", 31, timestamp.Add(time.Millisecond), "first"),
					),
					newMergedXIDEvent(
						newObservedXIDEvent("GPU-1", 31, timestamp.Add(8*time.Millisecond), 2, 2),
						newDriverXIDEvent("GPU-1", 31, timestamp.Add(7*time.Millisecond), "second"),
					),
				},
			}},
		},
		{
			name: "maximizes matches for repeated events",
			steps: []xidMergerTestStep{{
				queryTime: timestamp.Add(time.Second),
				nvmlEvents: []observedDeviceEvent{
					newObservedXIDEvent("GPU-1", 31, timestamp, 1, 1),
					newObservedXIDEvent("GPU-1", 31, timestamp.Add(100*time.Millisecond), 2, 2),
				},
				driverEvents: []model.DriverEvent{
					newDriverXIDEvent("GPU-1", 31, timestamp.Add(90*time.Millisecond), "first"),
					newDriverXIDEvent("GPU-1", 31, timestamp.Add(200*time.Millisecond), "second"),
				},
				driverEventsEnabled: true,
				expectedLatest: []xidEvent{
					newMergedXIDEvent(
						newObservedXIDEvent("GPU-1", 31, timestamp, 1, 1),
						newDriverXIDEvent("GPU-1", 31, timestamp.Add(90*time.Millisecond), "first"),
					),
					newMergedXIDEvent(
						newObservedXIDEvent("GPU-1", 31, timestamp.Add(100*time.Millisecond), 2, 2),
						newDriverXIDEvent("GPU-1", 31, timestamp.Add(200*time.Millisecond), "second"),
					),
				},
			}},
		},
		{
			name: "matches across refreshes with NVML first",
			steps: []xidMergerTestStep{
				{
					queryTime: timestamp.Add(5 * time.Millisecond),
					nvmlEvents: []observedDeviceEvent{
						newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
					},
					driverEventsEnabled: true,
					expectedPendingNVML: []xidEvent{
						newNVMLXIDEvent(newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2)),
					},
				},
				{
					queryTime: timestamp.Add(8 * time.Millisecond),
					driverEvents: []model.DriverEvent{
						newDriverXIDEvent("GPU-1", 31, timestamp.Add(4*time.Millisecond), "later"),
					},
					driverEventsEnabled: true,
					expectedLatest: []xidEvent{
						newMergedXIDEvent(
							newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
							newDriverXIDEvent("GPU-1", 31, timestamp.Add(4*time.Millisecond), "later"),
						),
					},
				},
				{
					queryTime:           timestamp.Add(time.Second),
					driverEventsEnabled: true,
				},
			},
		},
		{
			name: "matches across refreshes with driver first",
			steps: []xidMergerTestStep{
				{
					queryTime: timestamp.Add(5 * time.Millisecond),
					driverEvents: []model.DriverEvent{
						newDriverXIDEvent("GPU-1", 31, timestamp, "first"),
					},
					driverEventsEnabled: true,
					expectedPendingDriver: []xidEvent{
						newDriverOnlyXIDEvent(newDriverXIDEvent("GPU-1", 31, timestamp, "first")),
					},
				},
				{
					queryTime: timestamp.Add(8 * time.Millisecond),
					nvmlEvents: []observedDeviceEvent{
						newObservedXIDEvent("GPU-1", 31, timestamp.Add(4*time.Millisecond), 1, 2),
					},
					driverEventsEnabled: true,
					expectedLatest: []xidEvent{
						newMergedXIDEvent(
							newObservedXIDEvent("GPU-1", 31, timestamp.Add(4*time.Millisecond), 1, 2),
							newDriverXIDEvent("GPU-1", 31, timestamp, "first"),
						),
					},
				},
			},
		},
		{
			name: "retains cutoff boundary and finalizes older event",
			steps: []xidMergerTestStep{
				{
					queryTime: timestamp.Add(xidEventMergeWindow),
					nvmlEvents: []observedDeviceEvent{
						newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
					},
					driverEventsEnabled: true,
					expectedPendingNVML: []xidEvent{
						newNVMLXIDEvent(newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2)),
					},
				},
				{
					queryTime:           timestamp.Add(xidEventMergeWindow + time.Nanosecond),
					driverEventsEnabled: true,
					expectedLatest: []xidEvent{
						newNVMLXIDEvent(newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2)),
					},
				},
			},
		},
		{
			name: "finalizes immediately when driver events are disabled",
			steps: []xidMergerTestStep{{
				queryTime: timestamp,
				nvmlEvents: []observedDeviceEvent{
					newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2),
				},
				expectedLatest: []xidEvent{
					newNVMLXIDEvent(newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2)),
				},
			}},
		},
		{
			name: "ignores non-XID events",
			steps: []xidMergerTestStep{{
				queryTime: timestamp.Add(time.Second),
				nvmlEvents: []observedDeviceEvent{{
					DeviceEventData: ddnvml.DeviceEventData{
						DeviceUUID: "GPU-1",
						EventType:  nvml.EventMigConfigChange,
					},
					ObservedAt: timestamp,
				}},
				driverEvents: []model.DriverEvent{{
					DeviceUUID: "GPU-1",
					Timestamp:  timestamp,
					Type:       "other",
				}},
				driverEventsEnabled: true,
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merger := newXIDEventMerger(xidEventMergeWindow)
			for _, step := range test.steps {
				merger.Refresh(step.queryTime, step.nvmlEvents, step.driverEvents, step.driverEventsEnabled)
				assert.Equal(t, step.expectedLatest, merger.latest)
				assert.Equal(t, step.expectedPendingNVML, merger.pendingNVML)
				assert.Equal(t, step.expectedPendingDriver, merger.pendingDriver)
			}
		})
	}
}

func TestXIDEventToSampleIncludesStructuredTags(t *testing.T) {
	timestamp := time.Unix(100, 0)
	pid := uint64(123)
	linkID := uint64(4)
	partition := uint64(2)
	residualDRAM := int64(3)
	previousCode := uint64(0)
	currentCode := uint64(2)
	driverEvent := newDriverXIDEvent("GPU-1", 31, timestamp, "raw message")
	driverEvent.NvidiaXid.ProcessID = &pid
	driverEvent.NvidiaXid.ProcessName = "gpu-burner"
	driverEvent.NvidiaXid.Channel = "0x10"
	driverEvent.NvidiaXid.MMUFault = &model.NvidiaXidMMUFault{
		Interrupt:    "0x20",
		Engine:       "GRAPHICS",
		EngineClient: "GPCCLIENT",
		FaultAddress: "0xdeadbeef",
		FaultType:    "FAULT_PTE",
		AccessType:   "VIRT_READ",
	}
	driverEvent.NvidiaXid.NVLinkFault = &model.NvidiaXidNVLinkFault{
		Subcode:          "0x1",
		Fatal:            true,
		CrossContainment: true,
		Injected:         true,
		LinkID:           &linkID,
		IntrInfo:         "0x2",
		ErrorStatus:      "0x3",
		ErrorDebugData:   []string{"0x4"},
	}
	driverEvent.NvidiaXid.MemoryFault = &model.NvidiaXidMemoryFault{
		PhysicalAddress:      "0x1000",
		RowAddress:           "0x2000",
		RowRemapperSite:      "site-a",
		Partition:            &partition,
		Location:             "HBM",
		FBPA:                 &partition,
		InterruptStormSource: "dram",
		ResidualDRAM:         &residualDRAM,
	}
	driverEvent.NvidiaXid.Repair = &model.NvidiaXidRepair{
		Target:             "row",
		TargetIndex:        &partition,
		Container:          "FBPA",
		ContainerIndex:     &partition,
		Activation:         "node_reboot",
		NodeRebootRequired: true,
		Failed:             true,
		FailureReason:      "no_spare_channels",
		SpareSource:        "same_gpc",
		MIGMode:            true,
	}
	driverEvent.NvidiaXid.RecoveryAction = &model.NvidiaXidRecoveryAction{
		PreviousCode:  &previousCode,
		PreviousLabel: "none",
		CurrentCode:   &currentCode,
		CurrentLabel:  "reset",
	}
	nvmlEvent := newObservedXIDEvent("GPU-1", 31, timestamp, 5, 6)

	sample, ok := (xidEvent{
		DeviceUUID:  "GPU-1",
		XIDCode:     31,
		Timestamp:   timestamp,
		NVMLEvent:   &nvmlEvent,
		DriverEvent: &driverEvent,
	}).toSample().(*Event)
	require.True(t, ok)

	assert.Equal(t, "XID 31 error on GPU-1", sample.event.Title)
	assert.Equal(t, "raw message", sample.event.Text)
	assert.Equal(t, timestamp, sample.occurredAt)
	assert.ElementsMatch(t, []string{
		"xid_code:31",
		"origin:hardware",
		"event_source:nvml",
		"gpu_instance_id:5",
		"compute_instance_id:6",
		"event_source:kmsg",
		"pid:123",
		"process_name:gpu-burner",
		"channel:0x10",
		"interrupt:0x20",
		"engine:GRAPHICS",
		"engine_client:GPCCLIENT",
		"fault_type:FAULT_PTE",
		"access_type:VIRT_READ",
		"nvlink_subcode:0x1",
		"nvlink_fatal:true",
		"nvlink_cross_containment:true",
		"nvlink_injected:true",
		"nvlink_link_id:4",
		"memory_partition:2",
		"memory_location:HBM",
		"interrupt_storm_source:dram",
		"residual_dram:3",
		"row_remapper_site:site-a",
		"fbpa:2",
		"repair_target:row",
		"repair_target_index:2",
		"repair_container:FBPA",
		"repair_container_index:2",
		"repair_activation:node_reboot",
		"node_reboot_required:true",
		"repair_failed:true",
		"repair_failure_reason:no_spare_channels",
		"repair_spare_source:same_gpc",
		"repair_mig_mode:true",
		"recovery_previous_code:0",
		"recovery_previous_label:none",
		"recovery_current_code:2",
		"recovery_current_label:reset",
	}, sample.tags)
}

func TestXIDEventToSampleFallsBackToPCIBusIDWhenDeviceIsUnresolved(t *testing.T) {
	// A GPU that has fallen off the PCIe bus (Xid 79) has no resolvable UUID, so the PCI
	// bus ID has to carry the device identity through the title, aggregation key and tags.
	timestamp := time.Unix(100, 0)
	driverEvent := newDriverXIDEvent("", 79, timestamp, "GPU has fallen off the bus")
	driverEvent.PCIBusID = "0000:97:00.0"

	sample, ok := newDriverOnlyXIDEvent(driverEvent).toSample().(*Event)
	require.True(t, ok)

	assert.Equal(t, "XID 79 error on 0000:97:00.0", sample.event.Title)
	assert.Equal(t, "0000:97:00.0", sample.event.AggregationKey)
	assert.Contains(t, sample.tags, "pci_bus_id:0000:97:00.0")
}

func TestXIDEventToSampleKeepsUUIDWhenBothIdentifiersArePresent(t *testing.T) {
	timestamp := time.Unix(100, 0)
	driverEvent := newDriverXIDEvent("GPU-1", 31, timestamp, "raw message")
	driverEvent.PCIBusID = "0000:35:00.0"

	sample, ok := newDriverOnlyXIDEvent(driverEvent).toSample().(*Event)
	require.True(t, ok)

	assert.Equal(t, "XID 31 error on GPU-1", sample.event.Title)
	assert.Equal(t, "GPU-1", sample.event.AggregationKey)
	assert.Contains(t, sample.tags, "pci_bus_id:0000:35:00.0")
}

func TestXIDEventToSampleOmitsUnsetNVLinkFlags(t *testing.T) {
	// eventTagValue drops false booleans, so an observed nonfatal fault carries neither
	// nvlink_fatal nor nvlink_injected. Their presence is the signal worth filtering on.
	timestamp := time.Unix(100, 0)
	driverEvent := newDriverXIDEvent("GPU-1", 146, timestamp, "raw message")
	driverEvent.NvidiaXid.NVLinkFault = &model.NvidiaXidNVLinkFault{
		Subcode:          "TLW_RX_PIPE1",
		Fatal:            false,
		CrossContainment: false,
		Injected:         false,
	}

	sample, ok := newDriverOnlyXIDEvent(driverEvent).toSample().(*Event)
	require.True(t, ok)

	assert.Contains(t, sample.tags, "nvlink_subcode:TLW_RX_PIPE1")
	assert.NotContains(t, sample.tags, "nvlink_fatal:false")
	assert.NotContains(t, sample.tags, "nvlink_injected:false")
	assert.NotContains(t, sample.tags, "nvlink_cross_containment:false")
}

func TestXIDEventToSampleUsesNVMLFallbackText(t *testing.T) {
	nvmlEvent := newObservedXIDEvent("GPU-1", 43, time.Unix(100, 0), 0, 0)
	sample, ok := (xidEvent{
		DeviceUUID: "GPU-1",
		XIDCode:    43,
		Timestamp:  time.Unix(100, 0),
		NVMLEvent:  &nvmlEvent,
	}).toSample().(*Event)
	require.True(t, ok)

	assert.Equal(t, "NVIDIA XID 43 was reported by NVML; no kernel message was available.", sample.event.Text)
}

func newObservedXIDEvent(deviceUUID string, xidCode uint64, timestamp time.Time, gpuInstanceID, computeInstanceID uint32) observedDeviceEvent {
	return observedDeviceEvent{
		DeviceEventData: ddnvml.DeviceEventData{
			DeviceUUID:        deviceUUID,
			EventType:         nvml.EventTypeXidCriticalError,
			EventData:         xidCode,
			GPUInstanceID:     gpuInstanceID,
			ComputeInstanceID: computeInstanceID,
		},
		ObservedAt: timestamp,
	}
}

func newDriverXIDEvent(deviceUUID string, xidCode uint64, timestamp time.Time, message string) model.DriverEvent {
	return model.DriverEvent{
		DeviceUUID: deviceUUID,
		Timestamp:  timestamp,
		Type:       model.DriverEventTypeNvidiaXid,
		NvidiaXid: &model.NvidiaXid{
			XidCode: xidCode,
			Message: message,
		},
	}
}

// newUnresolvedDriverXIDEvent builds a driver event for a GPU whose UUID could not be
// resolved through NVML, as happens once the device has left the PCIe bus.
func newUnresolvedDriverXIDEvent(pciBusID string, xidCode uint64, timestamp time.Time, message string) model.DriverEvent {
	event := newDriverXIDEvent("", xidCode, timestamp, message)
	event.PCIBusID = pciBusID
	return event
}

func newNVMLXIDEvent(event observedDeviceEvent) xidEvent {
	return xidEvent{
		DeviceUUID: event.DeviceUUID,
		XIDCode:    event.EventData,
		Timestamp:  event.ObservedAt,
		NVMLEvent:  &event,
	}
}

func newDriverOnlyXIDEvent(event model.DriverEvent) xidEvent {
	return xidEvent{
		// Mirrors convertDriverXIDEvents, which keys on DeviceKey so an unresolved device
		// stays identified by its PCI bus ID.
		DeviceUUID:  event.DeviceKey(),
		XIDCode:     event.NvidiaXid.XidCode,
		Timestamp:   event.Timestamp,
		DriverEvent: &event,
	}
}

func newMergedXIDEvent(nvmlEvent observedDeviceEvent, driverEvent model.DriverEvent) xidEvent {
	event := newDriverOnlyXIDEvent(driverEvent)
	event.NVMLEvent = &nvmlEvent
	return event
}
