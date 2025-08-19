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

func encode(t *testing.T, typ agentmodel.MessageType, body agentmodel.MessageBody) []byte {
	encoded, err := agentmodel.EncodeMessage(agentmodel.Message{
		Header: agentmodel.MessageHeader{
			Version:  agentmodel.MessageV3,
			Encoding: agentmodel.MessageEncodingZstdPB,
			Type:     typ,
		},
		Body: body,
	})
	require.NoError(t, err)

	return encoded
}

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

	return encode(t, agentmodel.TypeCollectorProc, &body)
}

// CollectorContainerPayload serializes an agentmodel.CollectorContainer into a []byte payload.
// It should use the same serialization steps as the process-agent's EncodePayload function.
func CollectorContainerPayload(t *testing.T) []byte {
	body := agentmodel.CollectorContainer{
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
		GroupId:           21052302,
		GroupSize:         1,
		ContainerHostType: agentmodel.ContainerHostType_notSpecified,
		Containers: []*agentmodel.Container{
			{
				Type:        "containerd",
				Id:          "3ae55dbd0022",
				CpuLimit:    150,
				MemoryLimit: 2147483648,
				State:       agentmodel.ContainerState_running,
				Created:     1694110925,
				Rbps:        0,
				Wbps:        820.14996,
				NetRcvdPs:   27.31859,
				NetSentPs:   28.41934,
				NetRcvdBps:  3986.3127,
				NetSentBps:  4577.415,
				UserPct:     0.8697474,
				SystemPct:   0.4873545,
				TotalPct:    1.3571019,
				MemRss:      723034112,
				MemCache:    76529664,
				Started:     1694110925,
				Tags: []string{
					"tags.datadoghq.com/service:foo",
					"kube_namespace:default",
					"helm.sh/chart:foo-7.12.88",
				},
				Addresses: []*agentmodel.ContainerAddr{
					{
						Ip:       "10.3.4.7",
						Port:     5555,
						Protocol: agentmodel.ConnectionType_tcp,
					},
					{
						Ip:       "10.3.4.7",
						Port:     9092,
						Protocol: agentmodel.ConnectionType_tcp,
					},
					{
						Ip:       "10.3.4.7",
						Port:     9093,
						Protocol: agentmodel.ConnectionType_tcp,
					},
				},
				ThreadCount: 85,
				ThreadLimit: 629145,
			},
			{
				Type:        "containerd",
				Id:          "87722d0e3401",
				CpuLimit:    0,
				MemoryLimit: 0,
				State:       agentmodel.ContainerState_running,
				Created:     1694106427,
				Rbps:        0,
				Wbps:        0,
				NetRcvdPs:   9.706397,
				NetSentPs:   9.106002,
				NetRcvdBps:  1351.6909,
				NetSentBps:  2699.179,
				UserPct:     0.043428876,
				SystemPct:   0.04623074,
				TotalPct:    0.08964961,
				MemRss:      5337088,
				MemCache:    28672,
				Started:     1694106427,
				Tags: []string{
					"kube_service:bar",
					"kube_container_name:bar",
					"kube_qos:Burstable",
				},
				Addresses: []*agentmodel.ContainerAddr{
					{
						Ip:       "10.3.4.2",
						Port:     53,
						Protocol: agentmodel.ConnectionType_tcp,
					},
					{
						Ip:       "10.3.4.2",
						Port:     53,
						Protocol: agentmodel.ConnectionType_tcp,
					},
				},
				ThreadCount: 12,
				ThreadLimit: 629145,
			},
			{
				Type:        "containerd",
				Id:          "2c5d2d4b323e",
				CpuLimit:    0,
				MemoryLimit: 31997952,
				State:       agentmodel.ContainerState_running,
				Created:     1695861251,
				Rbps:        0,
				Wbps:        409.86932,
				NetRcvdPs:   1.400915,
				NetSentPs:   1.400915,
				NetRcvdBps:  116.07581,
				NetSentBps:  127.08301,
				UserPct:     0,
				SystemPct:   0.006434228,
				TotalPct:    0.006434228,
				MemRss:      7897088,
				MemCache:    4096,
				Started:     1695861251,
				Tags: []string{
					"app:baz",
					"app.kubernetes.io/name:baz",
					"pod_phase:running",
				},
				Addresses: []*agentmodel.ContainerAddr{
					{
						Ip:       "10.3.4.43",
						Port:     19091,
						Protocol: agentmodel.ConnectionType_tcp,
					},
				},
				ThreadCount: 14,
				ThreadLimit: 629145,
			},
			{
				Type:        "containerd",
				Id:          "4b0dccc07078",
				CpuLimit:    0,
				MemoryLimit: 0,
				State:       agentmodel.ContainerState_running,
				Created:     1695835227,
				Rbps:        0,
				Wbps:        0,
				NetRcvdPs:   39.32505,
				NetSentPs:   37.023582,
				NetRcvdBps:  92929.3,
				NetSentBps:  9122.311,
				UserPct:     0.29582912,
				SystemPct:   0.28884465,
				TotalPct:    0.58466375,
				MemRss:      32538624,
				MemCache:    12173312,
				Started:     1695835227,
				Tags: []string{
					"image_name:gcr.io/datadoghq/agent",
					"app.kubernetes.io/instance:client",
					"kube_app_name:agent",
				},
				Addresses: []*agentmodel.ContainerAddr{
					{
						Ip:       "10.3.4.41",
						Port:     8126,
						Protocol: agentmodel.ConnectionType_tcp,
					},
				},
				ThreadCount: 19,
				ThreadLimit: 629145,
			},
			{
				Type:        "containerd",
				Id:          "721ded045acc",
				CpuLimit:    0,
				MemoryLimit: 262144000,
				State:       agentmodel.ContainerState_running,
				Created:     1694106425,
				Rbps:        0,
				Wbps:        0,
				NetRcvdPs:   137.0763,
				NetSentPs:   140.97847,
				NetRcvdBps:  26829.232,
				NetSentBps:  125406,
				UserPct:     0.08267163,
				SystemPct:   0.050781712,
				TotalPct:    0.13345334,
				MemRss:      25145344,
				MemCache:    40960,
				Started:     1694106425,
				Tags: []string{
					"image_id:gke.gcr.io/bits@sha256:9fe46fa83eee8",
					"kube_daemon_set:bits-gke",
					"component:bits-gke",
				},
				Addresses: []*agentmodel.ContainerAddr{
					{
						Ip:       "10.3.4.42",
						Port:     2021,
						Protocol: agentmodel.ConnectionType_tcp,
					},
				},
				ThreadCount: 17,
				ThreadLimit: 629145,
			},
			{
				Type:        "containerd",
				Id:          "d17335a90e9c",
				CpuLimit:    0,
				MemoryLimit: 125829120,
				State:       agentmodel.ContainerState_running,
				Created:     1694496783,
				Rbps:        0,
				Wbps:        0,
				NetRcvdPs:   137.07614,
				NetSentPs:   140.97832,
				NetRcvdBps:  26829.203,
				NetSentBps:  125405.86,
				UserPct:     0.12658055,
				SystemPct:   0.06475609,
				TotalPct:    0.19133663,
				MemRss:      34955264,
				MemCache:    32768,
				Started:     1694496783,
				Tags: []string{
					"kube_container_name:agent",
					"image_name:gke.gcr.io/datadog-agent",
					"kube_ownerref_kind:daemonset",
				},
				Addresses:   []*agentmodel.ContainerAddr{},
				ThreadCount: 7,
				ThreadLimit: 629145,
			},
		},
	}

	return encode(t, agentmodel.TypeCollectorContainer, &body)
}

// CollectorProcDiscoveryPayload serializes an agentmodel.CollectorProcDiscovery into a []byte
// payload.
// It should use the same serialization steps as the process-agent's EncodePayload function.
func CollectorProcDiscoveryPayload(t *testing.T) []byte {
	body := agentmodel.CollectorProcDiscovery{
		HostName:  "i-078e212",
		GroupId:   21052302,
		GroupSize: 1,
		ProcessDiscoveries: []*agentmodel.ProcessDiscovery{
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
				CreateTime: 1696620615000,
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
				CreateTime: 1696620615000,
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
				CreateTime: 1696620744000,
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
				CreateTime: 1696620618000,
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
				CreateTime: 1696620635000,
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
				CreateTime: 1696620616000,
			},
		},
	}

	return encode(t, agentmodel.TypeCollectorProcDiscovery, &body)
}
