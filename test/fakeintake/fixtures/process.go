// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fixtures contains test fixtures used by other fakeintake packages.
package fixtures

import (
	"testing"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/require"
)

// CollectorProcPayload serializes an agentmodel.CollectorProc into a []byte payload.
// It should use the same serialization steps as the process-agent's EncodePayload function.
func CollectorProcPayload(t *testing.T) []byte {
	body := agentmodel.CollectorProc{
		HostName:  "i-078e212",
		NetworkId: "vpc-09cfa8d",
		Info: &agentmodel.SystemInfo{
			Uuid: "3d1145c8-5ae8-497b-a8bb-f010d6745328",
			Os: &agentmodel.OSInfo{
				Name:          "linux",
				Platform:      "ubuntu",
				Family:        "debian",
				Version:       "22.04",
				KernelVersion: "6.2.0-1012-aws",
			},
			Cpus: []*agentmodel.CPUInfo{
				{
					Number:     0,
					Vendor:     "GenuineIntel",
					Family:     "6",
					Model:      "85",
					PhysicalId: "0",
					CoreId:     "0",
					Cores:      1,
					Mhz:        2499,
					CacheSize:  36608,
				},
				{
					Number:     1,
					Vendor:     "GenuineIntel",
					Family:     "6",
					Model:      "85",
					PhysicalId: "0",
					CoreId:     "0",
					Cores:      1,
					Mhz:        2499,
					CacheSize:  36608,
				},
			},
			TotalMemory: 4034580480,
		},
		GroupId:   21052302,
		GroupSize: 1,
		Processes: []*agentmodel.Process{
			{
				Pid:   446,
				NsPid: 446,
				Command: &agentmodel.Command{
					Args: []string{"/usr/bin/python3", "/usr/bin/networkd-dispatcher", "--run-startup-triggers"},
					Ppid: 1,
				},
				User: &agentmodel.ProcessUser{
					Name: "root",
				},
				Memory: &agentmodel.MemoryStat{
					Rss:    19521536,
					Vms:    33878016,
					Shared: 10616832,
					Text:   2822144,
					Data:   10883072,
				},
				Cpu: &agentmodel.CPUStat{
					LastCpu:    "cpu",
					NumThreads: 1,
					Nice:       20,
				},
				CreateTime: 1696620615000,
				State:      agentmodel.ProcessState_S,
				IoStat: &agentmodel.IOStat{
					ReadRate:       -1,
					WriteRate:      -1,
					ReadBytesRate:  -1,
					WriteBytesRate: -1,
				},
				VoluntaryCtxSwitches:   89,
				InvoluntaryCtxSwitches: 111,
			},
			{
				Pid:   494,
				NsPid: 494,
				Command: &agentmodel.Command{
					Args: []string{"/usr/sbin/chronyd", "-F", "1"},
					Ppid: 485,
				},
				User: &agentmodel.ProcessUser{
					Name: "_chrony",
					Uid:  114,
					Gid:  121,
				},
				Memory: &agentmodel.MemoryStat{
					Rss:    2035712,
					Vms:    10813440,
					Shared: 1572864,
					Text:   217088,
					Data:   602112,
				},
				Cpu: &agentmodel.CPUStat{
					LastCpu:    "cpu",
					NumThreads: 1,
					Nice:       20,
				},
				CreateTime: 1696620615000,
				State:      agentmodel.ProcessState_S,
				IoStat: &agentmodel.IOStat{
					ReadRate:       -1,
					WriteRate:      -1,
					ReadBytesRate:  -1,
					WriteBytesRate: -1,
				},
				VoluntaryCtxSwitches:   16,
				InvoluntaryCtxSwitches: 7,
			},
			{
				Pid:   2636,
				NsPid: 2636,
				Command: &agentmodel.Command{
					Args: []string{"/opt/datadog-agent/embedded/bin/process-agent", "--cfgpath=/etc/datadog-agent/datadog.yaml", "--sysprobe-config=/etc/datadog-agent/system-probe.yaml", "--pid=/opt/datadog-agent/run/process-agent.pid"},
					Cwd:  "/",
					Ppid: 1,
					Exe:  "/opt/datadog-agent/embedded/bin/process-agent",
				},
				User: &agentmodel.ProcessUser{
					Name: "dd-agent",
					Uid:  115,
					Gid:  122,
				},
				Memory: &agentmodel.MemoryStat{
					Rss:    112590848,
					Vms:    1411502080,
					Shared: 73400320,
					Text:   52719616,
					Data:   145231872,
				},
				Cpu: &agentmodel.CPUStat{
					LastCpu:    "cpu",
					TotalPct:   0.5002501,
					UserPct:    0.20010005,
					SystemPct:  0.30015007,
					NumThreads: 8,
					Nice:       20,
					UserTime:   2,
					SystemTime: 2,
				},
				CreateTime: 1696620744000,
				State:      agentmodel.ProcessState_S,
				IoStat: &agentmodel.IOStat{
					ReadRate:       -1,
					WriteRate:      -1,
					ReadBytesRate:  -1,
					WriteBytesRate: -1,
				},
				VoluntaryCtxSwitches:   12370,
				InvoluntaryCtxSwitches: 675,
			},
			{
				Pid:   712,
				NsPid: 712,
				Command: &agentmodel.Command{
					Args: []string{"(sd-pam)"},
					Ppid: 711,
				},
				User: &agentmodel.ProcessUser{
					Name: "ubuntu",
					Uid:  1000,
					Gid:  1000,
				},
				Memory: &agentmodel.MemoryStat{
					Rss:    5443584,
					Vms:    106156032,
					Shared: 1703936,
					Text:   917504,
					Data:   20062208,
				},
				Cpu: &agentmodel.CPUStat{
					LastCpu:    "cpu",
					NumThreads: 1,
					Nice:       20,
				},
				CreateTime: 1696620618000,
				State:      agentmodel.ProcessState_S,
				IoStat: &agentmodel.IOStat{
					ReadRate:       -1,
					WriteRate:      -1,
					ReadBytesRate:  -1,
					WriteBytesRate: -1,
				},
				VoluntaryCtxSwitches:   1,
				InvoluntaryCtxSwitches: 1,
			},
			{
				Pid:   1767,
				NsPid: 1767,
				Command: &agentmodel.Command{
					Args: []string{"/usr/libexec/packagekitd"},
					Ppid: 1,
				},
				User: &agentmodel.ProcessUser{
					Name: "root",
				},
				Memory: &agentmodel.MemoryStat{
					Rss:    20840448,
					Vms:    303075328,
					Shared: 17825792,
					Text:   159744,
					Data:   27336704,
				},
				Cpu: &agentmodel.CPUStat{
					LastCpu:    "cpu",
					NumThreads: 3,
					Nice:       20,
				},
				CreateTime: 1696620635000,
				State:      agentmodel.ProcessState_S,
				IoStat: &agentmodel.IOStat{
					ReadRate:       -1,
					WriteRate:      -1,
					ReadBytesRate:  -1,
					WriteBytesRate: -1,
				},
				VoluntaryCtxSwitches:   326,
				InvoluntaryCtxSwitches: 27,
			},
			{
				Pid:   520,
				NsPid: 520,
				Command: &agentmodel.Command{
					Args: []string{"/sbin/agetty", "-o", "-p -- \\u", "--noclear", "tty1", "linux"},
					Ppid: 1,
				},
				User: &agentmodel.ProcessUser{
					Name: "root",
				},
				Memory: &agentmodel.MemoryStat{
					Rss:    2359296,
					Vms:    6320128,
					Shared: 2228224,
					Text:   28672,
					Data:   372736,
				},
				Cpu: &agentmodel.CPUStat{
					LastCpu:    "cpu",
					NumThreads: 1,
					Nice:       20,
				},
				CreateTime: 1696620616000,
				State:      agentmodel.ProcessState_S,
				IoStat: &agentmodel.IOStat{
					ReadRate:       -1,
					WriteRate:      -1,
					ReadBytesRate:  -1,
					WriteBytesRate: -1,
				},
				VoluntaryCtxSwitches:   4,
				InvoluntaryCtxSwitches: 14,
			},
		},
	}

	encoded, err := agentmodel.EncodeMessage(agentmodel.Message{
		Header: agentmodel.MessageHeader{
			Version:  agentmodel.MessageV3,
			Encoding: agentmodel.MessageEncodingZstdPB,
			Type:     agentmodel.TypeCollectorProc,
		},
		Body: &body,
	})
	require.NoError(t, err)

	return encoded
}
