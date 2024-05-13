// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/portlist"
)

type testProc struct {
	pid     int
	cmdline []string
	env     []string
	cwd     string
	stat    procfs.ProcStat
}

func mockProc(
	ctrl *gomock.Controller,
	p testProc,
) proc {
	m := NewMockproc(ctrl)
	m.EXPECT().PID().Return(p.pid).AnyTimes()
	m.EXPECT().CmdLine().Return(p.cmdline, nil).AnyTimes()
	m.EXPECT().Environ().Return(p.env, nil).AnyTimes()
	m.EXPECT().Cwd().Return(p.cwd, nil).AnyTimes()
	m.EXPECT().Stat().Return(p.stat, nil).AnyTimes()
	return m
}

func TestServiceDiscoveryCheckLinux(t *testing.T) {
	startTime := time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC)
	procLaunched := startTime.Add(-1 * time.Hour)
	host := "test-host"
	cfgYaml := `ignore_processes: ["ignore-1", "ignore-2"]`

	type checkRun struct {
		aliveProcs []testProc
		openPorts  []portlist.Port
		time       time.Time
	}

	tests := []struct {
		name       string
		checkRun   []*checkRun
		wantEvents []*event
	}{
		{
			name: "basic",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						{
							pid:     99,
							cmdline: []string{"some-service"},
							env:     []string{},
							cwd:     "",
							stat: procfs.ProcStat{
								Starttime: uint64(procLaunched.Unix()),
							},
						},
						{
							pid:     100,
							cmdline: []string{"ignore-1"},
							env:     nil,
							cwd:     "",
							stat: procfs.ProcStat{
								Starttime: uint64(procLaunched.Unix()),
							},
						},
						{
							pid:     6,
							cmdline: []string{"sshd"},
							env:     nil,
							cwd:     "",
							stat: procfs.ProcStat{
								Starttime: uint64(procLaunched.Unix()),
							},
						},
					},
					openPorts: []portlist.Port{
						{
							Proto:   "tcp",
							Port:    8080,
							Process: "some-service",
							Pid:     99,
						},
						{
							Proto:   "tcp",
							Port:    8081,
							Process: "ignore-1",
							Pid:     100,
						},
						{
							Proto:   "tcp",
							Port:    22,
							Process: "sshd",
							Pid:     6,
						},
					},
					time: startTime,
				},
				{
					aliveProcs: []testProc{
						{
							pid:     99,
							cmdline: []string{"some-service"},
							env:     []string{},
							cwd:     "",
							stat: procfs.ProcStat{
								Starttime: uint64(procLaunched.Unix()),
							},
						},
						{
							pid:     100,
							cmdline: []string{"ignore-1"},
							env:     nil,
							cwd:     "",
							stat: procfs.ProcStat{
								Starttime: uint64(procLaunched.Unix()),
							},
						},
						{
							pid:     6,
							cmdline: []string{"sshd"},
							env:     nil,
							cwd:     "",
							stat: procfs.ProcStat{
								Starttime: uint64(procLaunched.Unix()),
							},
						},
					},
					openPorts: []portlist.Port{
						{
							Proto:   "tcp",
							Port:    8080,
							Process: "some-service",
							Pid:     99,
						},
						{
							Proto:   "tcp",
							Port:    8081,
							Process: "ignore-1",
							Pid:     100,
						},
						{
							Proto:   "tcp",
							Port:    22,
							Process: "sshd",
							Pid:     6,
						},
					},
					time: startTime.Add(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						{
							pid:     99,
							cmdline: []string{"some-service"},
							env:     []string{},
							cwd:     "",
							stat: procfs.ProcStat{
								Starttime: uint64(procLaunched.Unix()),
							},
						},
						{
							pid:     100,
							cmdline: []string{"ignore-1"},
							env:     nil,
							cwd:     "",
							stat: procfs.ProcStat{
								Starttime: uint64(procLaunched.Unix()),
							},
						},
						{
							pid:     6,
							cmdline: []string{"sshd"},
							env:     nil,
							cwd:     "",
							stat: procfs.ProcStat{
								Starttime: uint64(procLaunched.Unix()),
							},
						},
					},
					openPorts: []portlist.Port{
						{
							Proto:   "tcp",
							Port:    8080,
							Process: "some-service",
							Pid:     99,
						},
						{
							Proto:   "tcp",
							Port:    8081,
							Process: "ignore-1",
							Pid:     100,
						},
						{
							Proto:   "tcp",
							Port:    22,
							Process: "sshd",
							Pid:     6,
						},
					},
					time: startTime.Add(20 * time.Minute),
				},
				{
					aliveProcs: []testProc{},
					openPorts:  []portlist.Port{},
					time:       startTime.Add(21 * time.Minute),
				},
			},
			wantEvents: []*event{
				{
					RequestType: "start-service",
					ApiVersion:  "v2",
					Payload: &eventPayload{
						ApiVersion:          "v1",
						NamingSchemaVersion: "1",
						RequestType:         "start-service",
						ServiceName:         "some-service",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           procLaunched.Unix(),
						LastSeen:            startTime.Add(1 * time.Minute).Unix(),
						APMInstrumentation:  false,
					},
				},
				{
					RequestType: "heartbeat-service",
					ApiVersion:  "v2",
					Payload: &eventPayload{
						ApiVersion:          "v1",
						NamingSchemaVersion: "1",
						RequestType:         "heartbeat-service",
						ServiceName:         "some-service",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           procLaunched.Unix(),
						LastSeen:            startTime.Add(20 * time.Minute).Unix(),
						APMInstrumentation:  false,
					},
				},
				{
					RequestType: "end-service",
					ApiVersion:  "v2",
					Payload: &eventPayload{
						ApiVersion:          "v1",
						NamingSchemaVersion: "1",
						RequestType:         "end-service",
						ServiceName:         "some-service",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           procLaunched.Unix(),
						LastSeen:            startTime.Add(21 * time.Minute).Unix(),
						APMInstrumentation:  false,
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// check and mocks setup
			check := newCheck().(*Check)

			sender := mocksender.NewMockSender(check.ID())
			sender.SetupAcceptAll()

			err := check.Configure(
				sender.GetSenderManager(),
				integration.FakeConfigHash,
				integration.Data(cfgYaml),
				nil,
				"test",
			)
			require.NoError(t, err)
			require.NotNil(t, check.os)

			for _, cr := range tc.checkRun {
				var procs []proc
				for _, p := range cr.aliveProcs {
					procs = append(procs, mockProc(ctrl, p))
				}

				_, hostnameMock := hostnameinterface.NewMock(hostnameinterface.MockHostname(host))

				portPollerMock := NewMockportPoller(ctrl)
				portPollerMock.EXPECT().OpenPorts().Return(cr.openPorts, nil).Times(1)

				procfsMock := NewMockprocFS(ctrl)
				procfsMock.EXPECT().AllProcs().Return(procs, nil).Times(1)

				timerMock := NewMocktimer(ctrl)
				timerMock.EXPECT().Now().Return(cr.time).AnyTimes()

				// set mocks
				check.os.(*linuxImpl).procfs = procfsMock
				check.os.(*linuxImpl).portPoller = portPollerMock
				check.os.(*linuxImpl).time = timerMock
				check.os.(*linuxImpl).sender.time = timerMock
				check.os.(*linuxImpl).sender.hostname = hostnameMock

				err = check.Run()
				require.NoError(t, err)
			}

			sender.AssertNumberOfCalls(t, "EventPlatformEvent", len(tc.wantEvents))

			var gotEvents []*event
			for _, call := range sender.Calls {
				evType := call.Arguments.Get(1).(string)
				assert.Equal(t, "service-discovery", evType)

				raw := call.Arguments.Get(0).([]byte)
				var ev *event
				err = json.Unmarshal(raw, &ev)
				require.NoError(t, err)
				gotEvents = append(gotEvents, ev)
			}

			if diff := cmp.Diff(tc.wantEvents, gotEvents); diff != "" {
				t.Errorf("event platform events mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
