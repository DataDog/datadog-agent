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
			// Driver format: ECC_CHANNEL_REPAIR_PENDING_XID_MESSAGE_FMT.
			name:    "channel repair names its FBPA and activation",
			message: "NVRM: Xid (PCI:0000:00:1e): 160, Marking Channel 3 in FBPA 2 along with its pair for repair. Perform node reboot to activate repair.",
			expected: model.NvidiaXid{
				XidCode: 160,
				Message: "NVRM: Xid (PCI:0000:00:1e): 160, Marking Channel 3 in FBPA 2 along with its pair for repair. Perform node reboot to activate repair.",
				Repair: &model.NvidiaXidRepair{
					Target:             "channel",
					TargetIndex:        uint64Pointer(3),
					Container:          "FBPA",
					ContainerIndex:     uint64Pointer(2),
					Activation:         "node_reboot",
					NodeRebootRequired: true,
				},
			},
		},
		{
			// Driver format: ECC_LTS_REPAIR_PENDING_XID_MESSAGE_FMT. This form says LTS and
			// FPB, not "L2 slice" and FBPA, so the old pattern could never match it.
			name:    "LTS repair names its FPB",
			message: "NVRM: Xid (PCI:0000:00:1e): 160, Marking LTS 5 in FPB 1 along with its pair for repair. Perform node reboot to activate repair.",
			expected: model.NvidiaXid{
				XidCode: 160,
				Message: "NVRM: Xid (PCI:0000:00:1e): 160, Marking LTS 5 in FPB 1 along with its pair for repair. Perform node reboot to activate repair.",
				Repair: &model.NvidiaXidRepair{
					Target:             "lts",
					TargetIndex:        uint64Pointer(5),
					Container:          "FPB",
					ContainerIndex:     uint64Pointer(1),
					Activation:         "node_reboot",
					NodeRebootRequired: true,
				},
			},
		},
		{
			// The activation verb is a format argument, so it is not always a node reboot.
			name:    "repair activated by a GPU reset is not a node reboot",
			message: "NVRM: Xid (PCI:0000:00:1e): 160, Marking Channel 0 in FBPA 12 along with its pair for repair. Perform GPU reset to activate repair.",
			expected: model.NvidiaXid{
				XidCode: 160,
				Message: "NVRM: Xid (PCI:0000:00:1e): 160, Marking Channel 0 in FBPA 12 along with its pair for repair. Perform GPU reset to activate repair.",
				Repair: &model.NvidiaXidRepair{
					Target:         "channel",
					TargetIndex:    uint64Pointer(0),
					Container:      "FBPA",
					ContainerIndex: uint64Pointer(12),
					Activation:     "gpu_reset",
				},
			},
		},
		{
			name:    "channel repair failure",
			message: "NVRM: Xid (PCI:0000:00:1e): 161, Repairing Channel failed as there are no more spare channels.",
			expected: model.NvidiaXid{
				XidCode: 161,
				Message: "NVRM: Xid (PCI:0000:00:1e): 161, Repairing Channel failed as there are no more spare channels.",
				Repair: &model.NvidiaXidRepair{
					Target:        "channel",
					Failed:        true,
					FailureReason: "no_spare_channels",
				},
			},
		},
		{
			name:    "LTS repair failure",
			message: "NVRM: Xid (PCI:0000:00:1e): 161, Repairing LTS failed as there are no more spare L2 slices.",
			expected: model.NvidiaXid{
				XidCode: 161,
				Message: "NVRM: Xid (PCI:0000:00:1e): 161, Repairing LTS failed as there are no more spare L2 slices.",
				Repair: &model.NvidiaXidRepair{
					Target:        "lts",
					Failed:        true,
					FailureReason: "no_spare_l2_slices",
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
			// Driver format: ECC_ROW_REMAP_PENDING_INTR_XID_MESSAGE_FMT. The address is
			// parenthesised, which the bare "row 0x…" pattern never matched.
			name:    "row remap pending names the new row",
			message: "NVRM: Xid (PCI:0000:00:1e): 63, Row Remapper: New row (0x0000000000abcdef) marked for remapping, reset gpu to activate.",
			expected: model.NvidiaXid{
				XidCode: 63,
				Message: "NVRM: Xid (PCI:0000:00:1e): 63, Row Remapper: New row (0x0000000000abcdef) marked for remapping, reset gpu to activate.",
				Repair: &model.NvidiaXidRepair{
					Target:  "row",
					Address: "0x0000000000abcdef",
				},
			},
		},
		{
			name:    "row remap failure names its cause",
			message: "NVRM: Xid (PCI:0000:00:1e): 64, Row Remapper Error: (0x0000000000abcdef) - Row Remapping table is full",
			expected: model.NvidiaXid{
				XidCode: 64,
				Message: "NVRM: Xid (PCI:0000:00:1e): 64, Row Remapper Error: (0x0000000000abcdef) - Row Remapping table is full",
				Repair: &model.NvidiaXidRepair{
					Target:        "row",
					Address:       "0x0000000000abcdef",
					Failed:        true,
					FailureReason: "row_remapping_table_is_full",
				},
			},
		},
		{
			// The condition the brief called out: a remap attempted on a row already pending.
			name:    "row remap failure on an already pending row",
			message: "NVRM: Xid (PCI:0000:00:1e): 64, Row Remapper: (0x0000000000abcdef) - Attempting to remap a row that is already pending remapping. Remapping will occur when the GPU is reset",
			expected: model.NvidiaXid{
				XidCode: 64,
				Message: "NVRM: Xid (PCI:0000:00:1e): 64, Row Remapper: (0x0000000000abcdef) - Attempting to remap a row that is already pending remapping. Remapping will occur when the GPU is reset",
				Repair: &model.NvidiaXidRepair{
					Target:        "row",
					Address:       "0x0000000000abcdef",
					Failed:        true,
					FailureReason: "attempting_to_remap_a_row_that_is_already_pending_remapping",
				},
			},
		},
		{
			name:    "DRAM retirement failure names its cause and address",
			message: "NVRM: Xid (PCI:0000:00:1e): 64, DRAM Retirement failed due to no spare for retirement at 0x0000000000001234",
			expected: model.NvidiaXid{
				XidCode: 64,
				Message: "NVRM: Xid (PCI:0000:00:1e): 64, DRAM Retirement failed due to no spare for retirement at 0x0000000000001234",
				Repair: &model.NvidiaXidRepair{
					Failed:        true,
					FailureReason: "no_spare_for_retirement",
					Address:       "0x0000000000001234",
				},
			},
		},
		{
			name:    "TPC retired with a spare from the same GPC",
			message: "NVRM: Xid (PCI:0000:00:1e): 156, Retiring TPC 4 from GPC 2 with a spare from the same GPC.",
			expected: model.NvidiaXid{
				XidCode: 156,
				Message: "NVRM: Xid (PCI:0000:00:1e): 156, Retiring TPC 4 from GPC 2 with a spare from the same GPC.",
				Repair: &model.NvidiaXidRepair{
					Target:         "tpc",
					TargetIndex:    uint64Pointer(4),
					Container:      "GPC",
					ContainerIndex: uint64Pointer(2),
					SpareSource:    "same_gpc",
				},
			},
		},
		{
			name:    "TPC retired with a spare from a different GPC",
			message: "NVRM: Xid (PCI:0000:00:1e): 156, Retiring TPC 4 from GPC 2 with a TPC from a different GPC.",
			expected: model.NvidiaXid{
				XidCode: 156,
				Message: "NVRM: Xid (PCI:0000:00:1e): 156, Retiring TPC 4 from GPC 2 with a TPC from a different GPC.",
				Repair: &model.NvidiaXidRepair{
					Target:         "tpc",
					TargetIndex:    uint64Pointer(4),
					Container:      "GPC",
					ContainerIndex: uint64Pointer(2),
					SpareSource:    "different_gpc",
				},
			},
		},
		{
			name:    "TPC retirement failure",
			message: "NVRM: Xid (PCI:0000:00:1e): 157, Unable to retire TPC 7 from GPC 3 as there are no spare TPCs available.",
			expected: model.NvidiaXid{
				XidCode: 157,
				Message: "NVRM: Xid (PCI:0000:00:1e): 157, Unable to retire TPC 7 from GPC 3 as there are no spare TPCs available.",
				Repair: &model.NvidiaXidRepair{
					Target:         "tpc",
					TargetIndex:    uint64Pointer(7),
					Container:      "GPC",
					ContainerIndex: uint64Pointer(3),
					Failed:         true,
					FailureReason:  "no_spare_tpc",
				},
			},
		},
		{
			// MIG confines the spare search to the same GPC, which is why this variant exists.
			name:    "TPC retirement failure under MIG",
			message: "NVRM: Xid (PCI:0000:00:1e): 157, Unable to retire TPC 7 from GPC 3 in MIG mode as there are no spare TPCs in the same GPC.",
			expected: model.NvidiaXid{
				XidCode: 157,
				Message: "NVRM: Xid (PCI:0000:00:1e): 157, Unable to retire TPC 7 from GPC 3 in MIG mode as there are no spare TPCs in the same GPC.",
				Repair: &model.NvidiaXidRepair{
					Target:         "tpc",
					TargetIndex:    uint64Pointer(7),
					Container:      "GPC",
					ContainerIndex: uint64Pointer(3),
					Failed:         true,
					MIGMode:        true,
					FailureReason:  "no_spare_tpc",
				},
			},
		},
		{
			name:    "bank remap pending",
			message: "NVRM: Xid (PCI:0000:00:1e): 177, Bank Remapper: New bank marked for remapping, reset gpu to activate.",
			expected: model.NvidiaXid{
				XidCode: 177,
				Message: "NVRM: Xid (PCI:0000:00:1e): 177, Bank Remapper: New bank marked for remapping, reset gpu to activate.",
				Repair:  &model.NvidiaXidRepair{Target: "bank"},
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
					Location:     "DRAM",
				},
			},
		},
		{
			name:    "SRAM annotation carries location without a parsable field",
			message: "NVRM: Xid (PCI:0000:00:1e): 172, Uncorrectable SRAM error",
			expected: model.NvidiaXid{
				XidCode:     172,
				Message:     "NVRM: Xid (PCI:0000:00:1e): 172, Uncorrectable SRAM error",
				MemoryFault: &model.NvidiaXidMemoryFault{Location: "SRAM"},
			},
		},
		{
			name:    "DRAM single-bit error storm names its partition",
			message: "NVRM: Xid (PCI:0000:00:1e): 92, Disabling ECC single-bit error interrupts in framebuffer at logical partition 3, due to high error rate",
			expected: model.NvidiaXid{
				XidCode: 92,
				Message: "NVRM: Xid (PCI:0000:00:1e): 92, Disabling ECC single-bit error interrupts in framebuffer at logical partition 3, due to high error rate",
				MemoryFault: &model.NvidiaXidMemoryFault{
					Partition:            uint64Pointer(3),
					InterruptStormSource: "dram",
				},
			},
		},
		{
			// The SM variant has no partition, and SRAM has no row remapping to absorb the
			// errors, so the two storms are not interchangeable.
			name:    "SM single-bit error storm has no partition",
			message: "NVRM: Xid (PCI:0000:00:1e): 92, SM SBE interrupt storm detected",
			expected: model.NvidiaXid{
				XidCode:     92,
				Message:     "NVRM: Xid (PCI:0000:00:1e): 92, SM SBE interrupt storm detected",
				MemoryFault: &model.NvidiaXidMemoryFault{InterruptStormSource: "sm"},
			},
		},
		{
			name:    "residual uncorrectable error counts per unit",
			message: "NVRM: Xid (PCI:0003:00:05): 140, pid='<unknown>', name=<unknown>, An uncorrectable ECC error detected (possible firmware handling failure) DRAM:2, LTC:0, MMU:0, PCIE:0",
			expected: model.NvidiaXid{
				XidCode:     140,
				Message:     "NVRM: Xid (PCI:0003:00:05): 140, pid='<unknown>', name=<unknown>, An uncorrectable ECC error detected (possible firmware handling failure) DRAM:2, LTC:0, MMU:0, PCIE:0",
				ProcessName: "<unknown>",
				MemoryFault: &model.NvidiaXidMemoryFault{
					ResidualDRAM: int64Pointer(2),
					ResidualLTC:  int64Pointer(0),
					ResidualMMU:  int64Pointer(0),
					ResidualPCIE: int64Pointer(0),
				},
			},
		},
		{
			// The driver prints these with %d and a large negative DRAM value is a known
			// counter-reporting artifact, so they must not be parsed as unsigned.
			name:    "residual counts survive the negative-counter artifact",
			message: "NVRM: Xid (PCI:0003:00:05): 140, An uncorrectable ECC error detected (possible firmware handling failure) DRAM:-2147483648, LTC:0, MMU:0, PCIE:0",
			expected: model.NvidiaXid{
				XidCode: 140,
				Message: "NVRM: Xid (PCI:0003:00:05): 140, An uncorrectable ECC error detected (possible firmware handling failure) DRAM:-2147483648, LTC:0, MMU:0, PCIE:0",
				MemoryFault: &model.NvidiaXidMemoryFault{
					ResidualDRAM: int64Pointer(-2147483648),
					ResidualLTC:  int64Pointer(0),
					ResidualMMU:  int64Pointer(0),
					ResidualPCIE: int64Pointer(0),
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

func int64Pointer(value int64) *int64 {
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
