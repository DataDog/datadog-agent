// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"cmp"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
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
	pythonCommandLine  = []string{"python", "-m", "foobar.main", "--", "--password", "secret",
		"--other-stuff", "--more-things", "--even-more",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"AAAAAAAAAAAAAAAAAAAAAAAAA",
		"--a-long-argument-total-over-max-length",
	}
	eventPythonCommandLine = []string{"python", "-m", "foobar.main", "--", "--password", "********",
		"--other-stuff", "--more-things", "--even-more",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"AAAAAAAAAAAAAAAAAAAAAAAAA",
		"--a-long-argument"}
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
	procPythonService = testProc{
		pid:     500,
		cmdline: pythonCommandLine,
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
	portTCP22 = model.Service{
		PID:   procSSHD.pid,
		Name:  "sshd",
		Ports: []uint16{22},
	}
	portTCP8080 = model.Service{
		PID:                procTestService1.pid,
		Name:               "test-service-1",
		Ports:              []uint16{8080},
		APMInstrumentation: string(apm.None),
	}
	portTCP8080DifferentPID = model.Service{
		PID:                procTestService1DifferentPID.pid,
		Name:               "test-service-1",
		Ports:              []uint16{8080},
		APMInstrumentation: string(apm.Injected),
	}
	portTCP8081 = model.Service{
		PID:   procIgnoreService1.pid,
		Name:  "ignore-1",
		Ports: []uint16{8081},
	}
	portTCP5000 = model.Service{
		PID:      procPythonService.pid,
		Name:     "python-service",
		Language: "python",
		Ports:    []uint16{5000},
	}
	portTCP5432 = model.Service{
		PID:   procTestService1Repeat.pid,
		Name:  "test-service-1",
		Ports: []uint16{5432},
	}
)

func mockProc(
	ctrl *gomock.Controller,
	p testProc,
) proc {
	m := NewMockproc(ctrl)
	m.EXPECT().PID().Return(p.pid).AnyTimes()
	m.EXPECT().CmdLine().Return(p.cmdline, nil).AnyTimes()
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
		cmp.Compare(ap.Ports[0], bp.Ports[0]),
		cmp.Compare(ap.PID, bp.PID),
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
	t.Setenv("DD_DISCOVERY_ENABLED", "true")

	type checkRun struct {
		aliveProcs   []testProc
		servicesResp *model.ServicesResponse
		time         time.Time
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
						procPythonService,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP5000,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procPythonService,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP5000,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procPythonService,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP5000,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(20 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procPythonService,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP5000,
					}},
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
						ServiceType:         "web_service",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						Ports:               []uint16{8080},
						PID:                 99,
						CommandLine:         []string{"test-service-1"},
						APMInstrumentation:  "none",
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						ServiceType:         "web_service",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						Ports:               []uint16{8080},
						PID:                 99,
						CommandLine:         []string{"test-service-1"},
						APMInstrumentation:  "none",
					},
				},
				{
					RequestType: "end-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						ServiceType:         "web_service",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						Ports:               []uint16{8080},
						PID:                 99,
						CommandLine:         []string{"test-service-1"},
						APMInstrumentation:  "none",
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "python-service",
						ServiceType:         "web_service",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						Ports:               []uint16{5000},
						PID:                 500,
						ServiceLanguage:     "python",
						CommandLine:         eventPythonCommandLine,
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "python-service",
						ServiceType:         "web_service",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						Ports:               []uint16{5000},
						PID:                 500,
						ServiceLanguage:     "python",
						CommandLine:         eventPythonCommandLine,
					},
				},
			},
		},
		{
			name: "repeated_service_name",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procTestService1Repeat,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					}},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procTestService1Repeat,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					}},
					time: calcTime(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procTestService1Repeat,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					}},
					time: calcTime(20 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
					}},
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
						ServiceType:         "db",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						Ports:               []uint16{5432},
						PID:                 101,
						CommandLine:         []string{"test-service-1"},
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						ServiceType:         "web_service",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						Ports:               []uint16{8080},
						PID:                 99,
						CommandLine:         []string{"test-service-1"},
						APMInstrumentation:  "none",
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						ServiceType:         "db",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						Ports:               []uint16{5432},
						PID:                 101,
						CommandLine:         []string{"test-service-1"},
					},
				},
				{
					RequestType: "end-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						ServiceType:         "db",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						Ports:               []uint16{5432},
						PID:                 101,
						CommandLine:         []string{"test-service-1"},
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						ServiceType:         "web_service",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						Ports:               []uint16{8080},
						PID:                 99,
						CommandLine:         []string{"test-service-1"},
						APMInstrumentation:  "none",
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
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1DifferentPID,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080DifferentPID,
					}},
					time: calcTime(21 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1DifferentPID,
					},
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080DifferentPID,
					}},
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
						ServiceType:         "web_service",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						Ports:               []uint16{8080},
						PID:                 99,
						CommandLine:         []string{"test-service-1"},
						APMInstrumentation:  "none",
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						ServiceType:         "web_service",
						HostName:            host,
						Env:                 "",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(22 * time.Minute).Unix(),
						Ports:               []uint16{8080},
						PID:                 102,
						CommandLine:         []string{"test-service-1"},
						APMInstrumentation:  "injected",
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
				mSysProbe := NewMocksystemProbeClient(ctrl)
				mSysProbe.EXPECT().GetDiscoveryServices().
					Return(cr.servicesResp, nil).
					Times(1)

				var procs []proc
				for _, p := range cr.aliveProcs {
					procs = append(procs, mockProc(ctrl, p))
				}

				_, mHostname := hostnameinterface.NewMock(hostnameinterface.MockHostname(host))

				mProcFS := NewMockprocFS(ctrl)
				mProcFS.EXPECT().AllProcs().Return(procs, nil).Times(1)

				mTimer := NewMocktimer(ctrl)
				mTimer.EXPECT().Now().Return(cr.time).AnyTimes()

				// set mocks
				check.os.(*linuxImpl).procfs = mProcFS
				check.os.(*linuxImpl).getSysProbeClient = func() (systemProbeClient, error) {
					return mSysProbe, nil
				}
				check.os.(*linuxImpl).time = mTimer
				check.os.(*linuxImpl).bootTime = bootTimeSeconds
				check.sender.hostname = mHostname

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

type errorProcFS struct{}

func (errorProcFS) AllProcs() ([]proc, error) {
	return nil, errors.New("procFS failure")
}

func Test_linuxImpl_errors(t *testing.T) {
	t.Setenv("DD_DISCOVERY_ENABLED", "true")

	// bad procFS
	{
		li := linuxImpl{
			procfs: errorProcFS{},
		}
		ds, err := li.DiscoverServices()
		if ds != nil {
			t.Error("expected nil discovery service")
		}
		var expected errWithCode
		if errors.As(err, &expected) {
			if expected.Code() != errorCodeProcfs {
				t.Errorf("expected error code procfs: %#v", expected)
			}
		} else {
			t.Error("expected errWithCode, got", err)
		}
	}
}

func TestTruncateCmdline(t *testing.T) {
	type testData struct {
		original []string
		result   []string
	}

	tests := []testData{
		{
			original: []string{},
			result:   nil,
		},
		{
			original: []string{"a", "b", "", "c", "d"},
			result:   []string{"a", "b", "c", "d"},
		},
		{
			original: []string{"x", strings.Repeat("A", maxCommandLine-1)},
			result:   []string{"x", strings.Repeat("A", maxCommandLine-1)},
		},
		{
			original: []string{strings.Repeat("A", maxCommandLine), "B"},
			result:   []string{strings.Repeat("A", maxCommandLine)},
		},
		{
			original: []string{strings.Repeat("A", maxCommandLine+1)},
			result:   []string{strings.Repeat("A", maxCommandLine)},
		},
		{
			original: []string{strings.Repeat("A", maxCommandLine-1), "", "B"},
			result:   []string{strings.Repeat("A", maxCommandLine-1), "B"},
		},
		{
			original: []string{strings.Repeat("A", maxCommandLine-1), "BCD"},
			result:   []string{strings.Repeat("A", maxCommandLine-1), "B"},
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.result, truncateCmdline(test.original))
	}
}
