// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"strings"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
)

func TestHumanFormatProcess(t *testing.T) {
	var msgs = []model.MessageBody{
		&model.CollectorProc{
			HostName: "foo",
			Info: &model.SystemInfo{
				TotalMemory: 16000000000,
				Cpus: []*model.CPUInfo{
					{}, {}, {}, {},
				},
			},
			Processes: []*model.Process{
				{
					Pid:         2,
					NsPid:       1002,
					ContainerId: "foo-container",
					Command: &model.Command{
						Exe:  "foo.exe",
						Comm: "foo",
						Args: []string{
							"1", "2", "3",
						},
						Cwd:  "/home/puppy",
						Ppid: 1,
					},
					User: &model.ProcessUser{
						Name: "root", Uid: 0, Gid: 1, Euid: 2, Egid: 3, Suid: 4, Sgid: 5,
					},
					CreateTime: 1609733040000,
					State:      model.ProcessState_R,
					Memory: &model.MemoryStat{
						Rss:    100,
						Vms:    200,
						Swap:   300,
						Shared: 400,
						Text:   500,
						Lib:    600,
						Data:   700,
						Dirty:  800,
					},
					Cpu: &model.CPUStat{
						TotalPct:   2.9999,
						SystemPct:  2.,
						UserPct:    1.,
						NumThreads: 100,
						Nice:       2,
					},
					OpenFdCount:            200,
					VoluntaryCtxSwitches:   1234,
					InvoluntaryCtxSwitches: 55,
					IoStat: &model.IOStat{
						ReadRate:       10.0,
						WriteRate:      30.0,
						ReadBytesRate:  100.0,
						WriteBytesRate: 200.0,
					},
				},
			},
			Containers: []*model.Container{
				{
					Id:          "foo-container-id",
					CpuLimit:    2.0,
					MemoryLimit: 300.0,
					State:       model.ContainerState_running,
					Health:      model.ContainerHealth_healthy,
					Created:     1609733040,
					Started:     1609733140,
					Tags:        []string{"image_name:foo", "image_tag:v1", "a:b", "c:d"},
					TotalPct:    10,
					SystemPct:   9,
					UserPct:     1,
					MemRss:      100,
					MemCache:    200,
					Rbps:        10,
					Wbps:        20,
					NetRcvdPs:   5,
					NetSentPs:   10,
					NetRcvdBps:  100,
					NetSentBps:  200,
					ThreadCount: 40,
					ThreadLimit: 100,
					Addresses: []*model.ContainerAddr{
						{
							Ip:       "192.168.0.102",
							Port:     10000,
							Protocol: model.ConnectionType_udp,
						},
					},
				},
			},
		},
	}

	w := &strings.Builder{}
	err := humanFormatProcess(msgs, w)
	assert.NoError(t, err)

	const expectHumanFormat = `Host Info
=========
Hostname: foo
Memory: 16 GB
CPUs: 4

Processes
=========
> PID: 2 NSPID: 1002 PPID: 1
  Container ID: foo-container
  Exe: foo.exe
  Command Name: foo
  Args: '1' '2' '3'
  Cwd: /home/puppy
  User: root Uid: 0 Gid: 1 Euid: 2 Egid: 3 Suid: 4 Sgid: 5
  Create Time: 2021-01-04T04:04:00Z
  State: R
  Memory:
    RSS:    100 B
    VMS:    200 B
    Swap:   300 B
    Shared: 400 B
    Text:   500 B
    Lib:    600 B
    Data:   700 B
    Dirty:  800 B
  CPU: Total: 3% System: 2% User: 1%
  Threads: 100
  Nice: 2
  Open Files: 200
  Context Switches: Voluntary: 1234 Involuntary: 55
  IO:
    Read:  100 B/s 10 Ops/s
    Write: 200 B/s 30 Ops/s

Containers
==========
> ID: foo-container-id
  CPU Limit:    2
  Memory Limit: 300 B
  State:  running
  Health: healthy
  Created: 2021-01-04T04:04:00Z
  Started: 2021-01-04T04:05:40Z
  CPU: Total: 10% System: 9% User: 1%
  Memory: RSS: 100 B Cache: 200 B
  IO:
    Read:  10 B/s
    Write: 20 B/s
  Net:
    Received: 100 B/s 5 Ops/s
    Sent:     200 B/s 10 Ops/s
  Tags: image_name:foo,image_tag:v1,a:b,c:d
  Thread Count: 40
  Thread Limit: 100
  Addresses:
    IP: 192.168.0.102 Port: 10000 udp

`
	assertEqualAnyLineBreak(t, expectHumanFormat, w.String())
}

func TestHumanFormatRealTimeProcess(t *testing.T) {
	msgs := []model.MessageBody{
		&model.CollectorRealTime{
			HostName:    "foo",
			TotalMemory: 16000000000,
			NumCpus:     4,
			Stats: []*model.ProcessStat{
				{
					Pid:          2,
					ContainerId:  "foo-container",
					CreateTime:   1609733040000,
					ProcessState: model.ProcessState_R,
					Memory: &model.MemoryStat{
						Rss:    100,
						Vms:    200,
						Swap:   300,
						Shared: 400,
						Text:   500,
						Lib:    600,
						Data:   700,
						Dirty:  800,
					},
					Cpu: &model.CPUStat{
						TotalPct:   2.95111,
						SystemPct:  2.,
						UserPct:    1.,
						NumThreads: 100,
						Nice:       2,
					},
					OpenFdCount:            200,
					VoluntaryCtxSwitches:   1234,
					InvoluntaryCtxSwitches: 45,
					IoStat: &model.IOStat{
						ReadRate:       10.0,
						WriteRate:      30.0,
						ReadBytesRate:  100.0,
						WriteBytesRate: 200.0,
					},
				},
			},
			ContainerStats: []*model.ContainerStat{
				{
					Id:          "foo-container-id",
					CpuLimit:    2.0,
					MemLimit:    300.0,
					State:       model.ContainerState_running,
					Health:      model.ContainerHealth_healthy,
					Started:     1609733140,
					TotalPct:    10,
					SystemPct:   9,
					UserPct:     1,
					MemRss:      100,
					MemCache:    200,
					Rbps:        10,
					Wbps:        20,
					NetRcvdPs:   5,
					NetSentPs:   10,
					NetRcvdBps:  100,
					NetSentBps:  200,
					ThreadCount: 40,
					ThreadLimit: 100,
				},
			},
		},
	}

	w := &strings.Builder{}
	err := humanFormatRealTimeProcess(msgs, w)
	assert.NoError(t, err)

	const expectHumanFormat = `Host Info
=========
Hostname: foo
Memory: 16 GB
CPUs: 4

RealTime Processes
==================
> PID: 2
  Container ID: foo-container
  Create Time: 2021-01-04T04:04:00Z
  State: R
  Memory:
    RSS:    100 B
    VMS:    200 B
    Swap:   300 B
    Shared: 400 B
    Text:   500 B
    Lib:    600 B
    Data:   700 B
    Dirty:  800 B
  CPU: Total: 2.95% System: 2% User: 1%
  Threads: 100
  Nice: 2
  Open Files: 200
  Context Switches: Voluntary: 1234 Involuntary: 45
  IO:
    Read:  100 B/s 10 Ops/s
    Write: 200 B/s 30 Ops/s

RealTime Containers
===================
> ID: foo-container-id
  CPU Limit:    2
  Memory Limit: 300 B
  State:  running
  Health: healthy
  Started: 2021-01-04T04:05:40Z
  CPU: Total: 10% System: 9% User: 1%
  Memory: RSS: 100 B Cache: 200 B
  IO:
    Read:  10 B/s
    Write: 20 B/s
  Net:
    Received: 100 B/s 5 Ops/s
    Sent:     200 B/s 10 Ops/s
  Thread Count: 40
  Thread Limit: 100

`
	assertEqualAnyLineBreak(t, expectHumanFormat, w.String())
}

func TestHumanFormatContainer(t *testing.T) {
	var msgs = []model.MessageBody{
		&model.CollectorContainer{
			HostName: "foo",
			Info: &model.SystemInfo{
				TotalMemory: 16000000000,
				Cpus: []*model.CPUInfo{
					{}, {}, {}, {},
				},
			},
			Containers: []*model.Container{
				{
					Id:          "foo-container-id",
					CpuLimit:    2.0,
					MemoryLimit: 300.0,
					State:       model.ContainerState_running,
					Health:      model.ContainerHealth_healthy,
					Created:     1609733040,
					Started:     1609733140,
					Tags:        []string{"image_name:foo", "image_tag:v1", "a:b", "c:d"},
					TotalPct:    10,
					SystemPct:   9,
					UserPct:     1,
					MemRss:      100,
					MemCache:    200,
					Rbps:        10,
					Wbps:        20,
					NetRcvdPs:   5,
					NetSentPs:   10,
					NetRcvdBps:  100,
					NetSentBps:  200,
					ThreadCount: 40,
					ThreadLimit: 100,
					Addresses: []*model.ContainerAddr{
						{
							Ip:       "192.168.0.102",
							Port:     10000,
							Protocol: model.ConnectionType_udp,
						},
					},
				},
				{
					Id:          "baz-container-id",
					CpuLimit:    3.0,
					MemoryLimit: 200.0,
					State:       model.ContainerState_running,
					Health:      model.ContainerHealth_unhealthy,
					Created:     1609733040,
					Started:     1609733240,
					Tags:        []string{"image_name:baz", "image_tag:v2", "a:b", "c:d"},
					TotalPct:    90,
					SystemPct:   80,
					UserPct:     10,
					MemRss:      10,
					MemCache:    20,
					Rbps:        10,
					Wbps:        20,
					NetRcvdPs:   5,
					NetSentPs:   10,
					NetRcvdBps:  100,
					NetSentBps:  200,
					ThreadCount: 40,
					ThreadLimit: 100,
					Addresses: []*model.ContainerAddr{
						{
							Ip:       "192.168.0.101",
							Port:     6732,
							Protocol: model.ConnectionType_tcp,
						},
					},
				},
			},
		},
	}

	w := &strings.Builder{}
	err := humanFormatContainer(msgs, w)
	assert.NoError(t, err)

	const expectHumanFormat = `Host Info
=========
Hostname: foo
Memory: 16 GB
CPUs: 4

Containers
==========
> ID: baz-container-id
  CPU Limit:    3
  Memory Limit: 200 B
  State:  running
  Health: unhealthy
  Created: 2021-01-04T04:04:00Z
  Started: 2021-01-04T04:07:20Z
  CPU: Total: 90% System: 80% User: 10%
  Memory: RSS: 10 B Cache: 20 B
  IO:
    Read:  10 B/s
    Write: 20 B/s
  Net:
    Received: 100 B/s 5 Ops/s
    Sent:     200 B/s 10 Ops/s
  Tags: image_name:baz,image_tag:v2,a:b,c:d
  Thread Count: 40
  Thread Limit: 100
  Addresses:
    IP: 192.168.0.101 Port: 6732 tcp
> ID: foo-container-id
  CPU Limit:    2
  Memory Limit: 300 B
  State:  running
  Health: healthy
  Created: 2021-01-04T04:04:00Z
  Started: 2021-01-04T04:05:40Z
  CPU: Total: 10% System: 9% User: 1%
  Memory: RSS: 100 B Cache: 200 B
  IO:
    Read:  10 B/s
    Write: 20 B/s
  Net:
    Received: 100 B/s 5 Ops/s
    Sent:     200 B/s 10 Ops/s
  Tags: image_name:foo,image_tag:v1,a:b,c:d
  Thread Count: 40
  Thread Limit: 100
  Addresses:
    IP: 192.168.0.102 Port: 10000 udp

`
	assertEqualAnyLineBreak(t, expectHumanFormat, w.String())
}

func TestHumanFormatRealTimeContainer(t *testing.T) {
	msgs := []model.MessageBody{
		&model.CollectorContainerRealTime{
			HostName:    "foo",
			TotalMemory: 16000000000,
			NumCpus:     4,
			Stats: []*model.ContainerStat{
				{
					Id:          "foo-container-id",
					CpuLimit:    2.0,
					MemLimit:    300.0,
					State:       model.ContainerState_running,
					Health:      model.ContainerHealth_healthy,
					Started:     1609733140,
					TotalPct:    10,
					SystemPct:   9,
					UserPct:     1,
					MemRss:      100,
					MemCache:    200,
					Rbps:        10,
					Wbps:        20,
					NetRcvdPs:   5,
					NetSentPs:   10,
					NetRcvdBps:  100,
					NetSentBps:  200,
					ThreadCount: 40,
					ThreadLimit: 100,
				},
				{
					Id:          "baz-container-id",
					CpuLimit:    3.0,
					MemLimit:    200.0,
					State:       model.ContainerState_running,
					Health:      model.ContainerHealth_unhealthy,
					Started:     1609733240,
					TotalPct:    90,
					SystemPct:   80,
					UserPct:     10,
					MemRss:      10,
					MemCache:    20,
					Rbps:        10,
					Wbps:        20,
					NetRcvdPs:   5,
					NetSentPs:   10,
					NetRcvdBps:  100,
					NetSentBps:  200,
					ThreadCount: 40,
					ThreadLimit: 100,
				},
			},
		},
	}

	w := &strings.Builder{}
	err := humanFormatRealTimeContainer(msgs, w)
	assert.NoError(t, err)

	const expectHumanFormat = `Host Info
=========
Hostname: foo
Memory: 16 GB
CPUs: 4

RealTime Containers
===================
> ID: baz-container-id
  CPU Limit:    3
  Memory Limit: 200 B
  State:  running
  Health: unhealthy
  Started: 2021-01-04T04:07:20Z
  CPU: Total: 90% System: 80% User: 10%
  Memory: RSS: 10 B Cache: 20 B
  IO:
    Read:  10 B/s
    Write: 20 B/s
  Net:
    Received: 100 B/s 5 Ops/s
    Sent:     200 B/s 10 Ops/s
  Thread Count: 40
  Thread Limit: 100
> ID: foo-container-id
  CPU Limit:    2
  Memory Limit: 300 B
  State:  running
  Health: healthy
  Started: 2021-01-04T04:05:40Z
  CPU: Total: 10% System: 9% User: 1%
  Memory: RSS: 100 B Cache: 200 B
  IO:
    Read:  10 B/s
    Write: 20 B/s
  Net:
    Received: 100 B/s 5 Ops/s
    Sent:     200 B/s 10 Ops/s
  Thread Count: 40
  Thread Limit: 100

`
	assertEqualAnyLineBreak(t, expectHumanFormat, w.String())
}

func TestHumanFormatProcessDiscovery(t *testing.T) {
	var msgs = []model.MessageBody{
		&model.CollectorProcDiscovery{
			HostName: "foo",
			ProcessDiscoveries: []*model.ProcessDiscovery{
				{
					Pid:   2,
					NsPid: 1002,
					Command: &model.Command{
						Exe: "foo.exe",
						Args: []string{
							"1", "2", "3",
						},
						Cwd:  "/home/puppy",
						Ppid: 1,
					},
					User: &model.ProcessUser{
						Name: "root", Uid: 0, Gid: 1, Euid: 2, Egid: 3, Suid: 4, Sgid: 5,
					},
					CreateTime: 1609733040000,
				},
			},
		},
	}

	w := &strings.Builder{}
	err := humanFormatProcessDiscovery(msgs, w)
	assert.NoError(t, err)

	const expectHumanFormat = `Host Info
=========
Hostname: foo

Process Discovery
=================
> PID: 2 NSPID: 1002 PPID: 1
  Exe: foo.exe
  Args: '1' '2' '3'
  Cwd: /home/puppy
  User: root Uid: 0 Gid: 1 Euid: 2 Egid: 3 Suid: 4 Sgid: 5
  Create Time: 2021-01-04T04:04:00Z

`
	assertEqualAnyLineBreak(t, expectHumanFormat, w.String())
}

func TestHumanFormatProcessEvents(t *testing.T) {
	msgs := []model.MessageBody{
		&model.CollectorProcEvent{
			Hostname: "foo",
			Events: []*model.ProcessEvent{
				{
					Type:           model.ProcEventType_exec,
					CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:10Z").UnixNano(),
					Pid:            42,
					ContainerId:    "abc8392",
					Command: &model.Command{
						Exe:  "/usr/bin/curl",
						Args: []string{"curl", "localhost:6062/debug/vars"},
						Ppid: 1,
					},
					User: &model.ProcessUser{
						Name: "user",
						Uid:  100,
						Gid:  100,
					},
					TypedEvent: &model.ProcessEvent_Exec{
						Exec: &model.ProcessExec{
							ForkTime: parseRFC3339Time(t, "2022-06-12T12:00:01Z").UnixNano(),
							ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:02Z").UnixNano(),
						},
					},
				},
			},
		},
		&model.CollectorProcEvent{
			Hostname: "foo",
			Events: []*model.ProcessEvent{
				{
					Type:           model.ProcEventType_exit,
					CollectionTime: parseRFC3339Time(t, "2022-06-12T12:00:20Z").UnixNano(),
					Pid:            42,
					ContainerId:    "abc8392",
					Command: &model.Command{
						Exe:  "/usr/bin/curl",
						Args: []string{"curl", "localhost:6062/debug/vars"},
						Ppid: 1,
					},
					User: &model.ProcessUser{
						Name: "user",
						Uid:  100,
						Gid:  100,
					},
					TypedEvent: &model.ProcessEvent_Exit{
						Exit: &model.ProcessExit{
							ExecTime: parseRFC3339Time(t, "2022-06-12T12:00:02Z").UnixNano(),
							ExitTime: parseRFC3339Time(t, "2022-06-12T12:00:12Z").UnixNano(),
						},
					},
				},
			},
		},
	}

	checkOut := &strings.Builder{}
	err := HumanFormatProcessEvents(msgs, checkOut, true)
	assert.NoError(t, err)

	eventsOut := &strings.Builder{}
	err = HumanFormatProcessEvents(msgs, eventsOut, false)
	assert.NoError(t, err)

	const expectCheckHumanFormat = `Host Info
=========
Hostname: foo

Process Lifecyle Events
=======================
> Type: exec
  Collection Time: 2022-06-12T12:00:10Z
  PID: 42 PPID: 1
  ContainerID: abc8392
  Exe: /usr/bin/curl
  Args: 'curl' 'localhost:6062/debug/vars'
  User: user Uid: 100 Gid: 100
  ForkTime: 2022-06-12T12:00:01Z
  ExecTime: 2022-06-12T12:00:02Z
> Type: exit
  Collection Time: 2022-06-12T12:00:20Z
  PID: 42 PPID: 1
  ContainerID: abc8392
  Exe: /usr/bin/curl
  Args: 'curl' 'localhost:6062/debug/vars'
  User: user Uid: 100 Gid: 100
  ExecTime: 2022-06-12T12:00:02Z
  ExitTime: 2022-06-12T12:00:12Z
  ExitCode: 0
`

	const expectEventsHumanFormat = `> Type: exec
  Collection Time: 2022-06-12T12:00:10Z
  PID: 42 PPID: 1
  ContainerID: abc8392
  Exe: /usr/bin/curl
  Args: 'curl' 'localhost:6062/debug/vars'
  User: user Uid: 100 Gid: 100
  ForkTime: 2022-06-12T12:00:01Z
  ExecTime: 2022-06-12T12:00:02Z
> Type: exit
  Collection Time: 2022-06-12T12:00:20Z
  PID: 42 PPID: 1
  ContainerID: abc8392
  Exe: /usr/bin/curl
  Args: 'curl' 'localhost:6062/debug/vars'
  User: user Uid: 100 Gid: 100
  ExecTime: 2022-06-12T12:00:02Z
  ExitTime: 2022-06-12T12:00:12Z
  ExitCode: 0
`
	assertEqualAnyLineBreak(t, expectCheckHumanFormat, checkOut.String())
	assertEqualAnyLineBreak(t, expectEventsHumanFormat, eventsOut.String())
}

// assertEqualAnyLineBreak is an assertion helper to compare strings ignoring the \r character
func assertEqualAnyLineBreak(t *testing.T, expected, actual string) {
	t.Helper()
	assert.Equal(t, expected, strings.Replace(actual, "\r\n", "\n", -1))
}
