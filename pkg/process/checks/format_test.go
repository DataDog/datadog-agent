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
	var msgs []model.MessageBody = []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{
				{
					Pid:         2,
					NsPid:       1002,
					ContainerId: "foo-container",
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
					CreateTime: 1609733040,
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
						TotalPct:   3.,
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
					Name:        "foo",
					Image:       "foo/foo:v1",
					CpuLimit:    2.0,
					MemoryLimit: 300.0,
					State:       model.ContainerState_running,
					Health:      model.ContainerHealth_healthy,
					Created:     1609733040,
					Started:     1609733140,
					Tags:        []string{"a:b", "c:d"},
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
	err := humanFormatProcess(msgs, w)
	assert.NoError(t, err)

	const expectHumanFormat = `Processes
=========
> PID: 2 NSPID: 1002 PPID: 1
  Container ID: foo-container
  Exe: foo.exe
  Args: '1' '2' '3'
  Cwd: /home/puppy
  User: root Uid: 0 Gid: 1 Euid: 2 Egid: 3 Suid: 4 Sgid: 5
  Create Time: 2021-01-04T04:04:00Z
  State: R
  Memory:
    Rss:    100
    Vms:    200
    Swap:   300
    Shared: 400
    Text:   500
    Lib:    600
    Data:   700
    Dirty:  800
  CPU: Total: 3% System: 2% User: 1%
  Threads: 100
  Nice: 2
  Open Files: 200
  Context Switches: Voluntary: 1234 Involuntary: 55
  IO:
    Read:  100 Bytes/s 10 Ops/s
    Write: 200 Bytes/s 30 Ops/s

Containers
==========
> ID: foo-container-id
  Name: foo
  Image: foo/foo:v1
  CPU Limit:    2
  Memory Limit: 300
  State:  running
  Health: healthy
  Created: 2021-01-04T04:04:00Z
  Started: 2021-01-04T04:05:40Z
  CPU: Total: 10% System: 9% User: 1%
  Memory: RSS: 100 Cache: 200
  IO:
    Read:  10 Bytes/s
    Write: 20 Bytes/s
  Net:
    Received: 100 Bytes/s 5 Ops/s
    Sent:     200 Bytes/s 10 Ops/s
  Tags: a:b,c:d
  Addresses:
  Thread Count: 40
  Thread Limit: 100

`
	assert.Equal(t, expectHumanFormat, w.String())
}

func TestHumanFormatRealTimeProcess(t *testing.T) {
	msgs := []model.MessageBody{
		&model.CollectorRealTime{
			Stats: []*model.ProcessStat{
				{
					Pid:          2,
					ContainerId:  "foo-container",
					CreateTime:   1609733040,
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
						TotalPct:   3.,
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

	const expectHumanFormat = `RealTime Processes
==================
> PID: 2
  Container ID: foo-container
  Create Time: 2021-01-04T04:04:00Z
  State: R
  Memory:
    Rss:    100
    Vms:    200
    Swap:   300
    Shared: 400
    Text:   500
    Lib:    600
    Data:   700
    Dirty:  800
  CPU: Total: 3% System: 2% User: 1%
  Threads: 100
  Nice: 2
  Open Files: 200
  Context Switches: Voluntary: 1234 Involuntary: 45
  IO:
    Read:  100 Bytes/s 10 Ops/s
    Write: 200 Bytes/s 30 Ops/s

RealTime Containers
===================
> ID: foo-container-id
  CPU Limit:    2
  Memory Limit: 300
  State:  running
  Health: healthy
  Started: 2021-01-04T04:05:40Z
  CPU: Total: 10% System: 9% User: 1%
  Memory: RSS: 100 Cache: 200
  IO:
    Read:  10 Bytes/s
    Write: 20 Bytes/s
  Net:
    Received: 100 Bytes/s 5 Ops/s
    Sent:     200 Bytes/s 10 Ops/s
  Thread Count: 40
  Thread Limit: 100

`
	assert.Equal(t, expectHumanFormat, w.String())
}

func TestHumanFormatContainer(t *testing.T) {
	var msgs []model.MessageBody = []model.MessageBody{
		&model.CollectorContainer{
			Containers: []*model.Container{
				{
					Id:          "foo-container-id",
					Name:        "foo",
					Image:       "foo/foo:v1",
					CpuLimit:    2.0,
					MemoryLimit: 300.0,
					State:       model.ContainerState_running,
					Health:      model.ContainerHealth_healthy,
					Created:     1609733040,
					Started:     1609733140,
					Tags:        []string{"a:b", "c:d"},
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
					Name:        "baz",
					Image:       "baz/baz:v1",
					CpuLimit:    3.0,
					MemoryLimit: 200.0,
					State:       model.ContainerState_running,
					Health:      model.ContainerHealth_unhealthy,
					Created:     1609733040,
					Started:     1609733240,
					Tags:        []string{"a:b", "c:d"},
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
	err := humanFormatContainer(msgs, w)
	assert.NoError(t, err)

	const expectHumanFormat = `Containers
==========
> ID: baz-container-id
  Name: baz
  Image: baz/baz:v1
  CPU Limit:    3
  Memory Limit: 200
  State:  running
  Health: unhealthy
  Created: 2021-01-04T04:04:00Z
  Started: 2021-01-04T04:07:20Z
  CPU: Total: 90% System: 80% User: 10%
  Memory: RSS: 10 Cache: 20
  IO:
    Read:  10 Bytes/s
    Write: 20 Bytes/s
  Net:
    Received: 100 Bytes/s 5 Ops/s
    Sent:     200 Bytes/s 10 Ops/s
  Tags: a:b,c:d
  Addresses:
  Thread Count: 40
  Thread Limit: 100
> ID: foo-container-id
  Name: foo
  Image: foo/foo:v1
  CPU Limit:    2
  Memory Limit: 300
  State:  running
  Health: healthy
  Created: 2021-01-04T04:04:00Z
  Started: 2021-01-04T04:05:40Z
  CPU: Total: 10% System: 9% User: 1%
  Memory: RSS: 100 Cache: 200
  IO:
    Read:  10 Bytes/s
    Write: 20 Bytes/s
  Net:
    Received: 100 Bytes/s 5 Ops/s
    Sent:     200 Bytes/s 10 Ops/s
  Tags: a:b,c:d
  Addresses:
  Thread Count: 40
  Thread Limit: 100
`
	assert.Equal(t, expectHumanFormat, w.String())
}

func TestHumanFormatRealTimeContainer(t *testing.T) {
	msgs := []model.MessageBody{
		&model.CollectorContainerRealTime{
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

	const expectHumanFormat = `RealTime Containers
===================
> ID: baz-container-id
  CPU Limit:    3
  Memory Limit: 200
  State:  running
  Health: unhealthy
  Started: 2021-01-04T04:07:20Z
  CPU: Total: 90% System: 80% User: 10%
  Memory: RSS: 10 Cache: 20
  IO:
    Read:  10 Bytes/s
    Write: 20 Bytes/s
  Net:
    Received: 100 Bytes/s 5 Ops/s
    Sent:     200 Bytes/s 10 Ops/s
  Thread Count: 40
  Thread Limit: 100
> ID: foo-container-id
  CPU Limit:    2
  Memory Limit: 300
  State:  running
  Health: healthy
  Started: 2021-01-04T04:05:40Z
  CPU: Total: 10% System: 9% User: 1%
  Memory: RSS: 100 Cache: 200
  IO:
    Read:  10 Bytes/s
    Write: 20 Bytes/s
  Net:
    Received: 100 Bytes/s 5 Ops/s
    Sent:     200 Bytes/s 10 Ops/s
  Thread Count: 40
  Thread Limit: 100
`
	assert.Equal(t, expectHumanFormat, w.String())
}

func TestHumanFormatProcessDiscovery(t *testing.T) {
	var msgs []model.MessageBody = []model.MessageBody{
		&model.CollectorProcDiscovery{
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
					CreateTime: 1609733040,
				},
			},
		},
	}

	w := &strings.Builder{}
	err := humanFormatProcessDiscovery(msgs, w)
	assert.NoError(t, err)

	const expectHumanFormat = `Process Discovery
=================
> PID: 2 NSPID: 1002 PPID: 1
  Exe: foo.exe
  Args: '1' '2' '3'
  Cwd: /home/puppy
  User: root Uid: 0 Gid: 1 Euid: 2 Egid: 3 Suid: 4 Sgid: 5
  Create Time: 2021-01-04T04:04:00Z
`
	assert.Equal(t, expectHumanFormat, w.String())
}
