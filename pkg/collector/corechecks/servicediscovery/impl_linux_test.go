// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"cmp"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/prometheus/procfs"
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

var (
	bootTimeSeconds = uint64(time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC).Unix())
	// procLaunched is number of clicks (100 per second) since bootTime when the process started
	// assume it's 12 hours later
	procLaunchedClicks = uint64((12 * time.Hour).Seconds()) * 100
)

var (
	procSSHD = testProc{
		pid:     6,
		cmdline: []string{"sshd"},
		env:     nil,
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procTestService1 = testProc{
		pid:     99,
		cmdline: []string{"test-service-1"},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procIgnoreService1 = testProc{
		pid:     100,
		cmdline: []string{"ignore-1"},
		env:     nil,
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procTestService1Repeat = testProc{
		pid:     101,
		cmdline: []string{"test-service-1"}, // same name as procTestService1
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procTestService1DifferentPID = testProc{
		pid:     102,
		cmdline: []string{"test-service-1"},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
)

var (
	portTCP22 = portlist.Port{
		Proto:   "tcp",
		Port:    22,
		Process: "sshd",
		Pid:     procSSHD.pid,
	}
	portTCP8080 = portlist.Port{
		Proto:   "tcp",
		Port:    8080,
		Process: "test-service-1",
		Pid:     procTestService1.pid,
	}
	portTCP8080DifferentPID = portlist.Port{
		Proto:   "tcp",
		Port:    8080,
		Process: "test-service-1",
		Pid:     procTestService1DifferentPID.pid,
	}
	portTCP8081 = portlist.Port{
		Proto:   "tcp",
		Port:    8081,
		Process: "ignore-1",
		Pid:     procIgnoreService1.pid,
	}
	portTCP5432 = portlist.Port{
		Proto:   "tcp",
		Port:    5432,
		Process: "test-service-1",
		Pid:     procTestService1Repeat.pid,
	}
)

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

func calcTime(additionalTime time.Duration) time.Time {
	unix := time.Unix(int64(bootTimeSeconds+(procLaunchedClicks/100)), 0)
	return unix.Add(additionalTime)
}

// cmpEvents is used to sort event slices in tests.
// It returns true if a is smaller than b, false otherwise.
func cmpEvents(a, b *event) bool {
	if a == nil || a.Payload == nil {
		return true
	}
	if b == nil || b.Payload == nil {
		return false
	}
	ap := a.Payload
	bp := b.Payload

	vals := []any{
		cmp.Compare(ap.LastSeen, bp.LastSeen),
		cmp.Compare(ap.ServiceName, bp.ServiceName),
		cmp.Compare(ap.ServiceType, bp.ServiceType),
		cmp.Compare(ap.ServiceLanguage, bp.ServiceLanguage),
	}
	for _, val := range vals {
		if val != 0 {
			return val == -1
		}
	}
	return false
}

func Test_linuxImpl(t *testing.T) {
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
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					},
					time: calcTime(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					},
					time: calcTime(20 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
					},
					openPorts: []portlist.Port{
						portTCP22,
					},
					time: calcTime(21 * time.Minute),
				},
			},
			wantEvents: []*event{
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "end-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
			},
		},
		{
			// TODO: ideally we would like to emit some sort of telemetry for this case.
			//  For now, we just test we send the correct events to EvP.
			name: "repeated_service_name",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procTestService1Repeat,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procTestService1Repeat,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					},
					time: calcTime(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procTestService1Repeat,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					},
					time: calcTime(20 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080,
					},
					time: calcTime(21 * time.Minute),
				},
			},
			wantEvents: []*event{
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "db",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "db",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "end-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "db",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
			},
		},
		{
			// in case we detect a service is restarted, we skip the stop event and send
			// another start event instead.
			name: "restart_service",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					},
					time: calcTime(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1DifferentPID,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080DifferentPID,
					},
					time: calcTime(21 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1DifferentPID,
					},
					openPorts: []portlist.Port{
						portTCP22,
						portTCP8080DifferentPID,
					},
					time: calcTime(22 * time.Minute),
				},
			},
			wantEvents: []*event{
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            host,
						Env:                 "",
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(22 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
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

			mSender := mocksender.NewMockSender(check.ID())
			mSender.SetupAcceptAll()

			err := check.Configure(
				mSender.GetSenderManager(),
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

				_, mHostname := hostnameinterface.NewMock(hostnameinterface.MockHostname(host))

				mPortPoller := NewMockportPoller(ctrl)
				mPortPoller.EXPECT().OpenPorts().Return(cr.openPorts, nil).Times(1)

				mProcFS := NewMockprocFS(ctrl)
				mProcFS.EXPECT().AllProcs().Return(procs, nil).Times(1)

				mTimer := NewMocktimer(ctrl)
				mTimer.EXPECT().Now().Return(cr.time).AnyTimes()

				// set mocks
				check.os.(*linuxImpl).procfs = mProcFS
				check.os.(*linuxImpl).portPoller = mPortPoller
				check.os.(*linuxImpl).time = mTimer
				check.os.(*linuxImpl).sender.hostname = mHostname
				check.os.(*linuxImpl).bootTime = bootTimeSeconds

				err = check.Run()
				require.NoError(t, err)
			}

			mSender.AssertNumberOfCalls(t, "EventPlatformEvent", len(tc.wantEvents))
			gotEvents := mockSenderEvents(t, mSender)

			diff := gocmp.Diff(tc.wantEvents, gotEvents, cmpopts.SortSlices(cmpEvents))

			if diff != "" {
				t.Errorf("event platform events mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
