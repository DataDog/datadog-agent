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
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
)

type testProc struct {
	pid int
	env []string
	cwd string
}

var (
	bootTimeSeconds = uint64(time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC).Unix())
	// procLaunched is number of clicks (100 per second) since bootTime when the process started
	// assume it's 12 hours later
	procLaunchedSeconds = bootTimeSeconds + uint64((12 * time.Hour).Seconds())
	pythonCommandLine   = []string{"python", "-m", "foobar.main"}
)

var (
	procSSHD = testProc{
		pid: 6,
		env: nil,
		cwd: "",
	}
	procTestService1 = testProc{
		pid: 99,
		env: []string{},
		cwd: "",
	}
	procPythonService = testProc{
		pid: 500,
		env: []string{},
		cwd: "",
	}
	procIgnoreService1 = testProc{
		pid: 100,
		env: nil,
		cwd: "",
	}
	procTestService1Repeat = testProc{
		pid: 101,
		env: []string{},
		cwd: "",
	}
	procTestService1DifferentPID = testProc{
		pid: 102,
		env: []string{},
		cwd: "",
	}
)

var (
	portTCP22 = model.Service{
		PID:           procSSHD.pid,
		GeneratedName: "sshd",
		Ports:         []uint16{22},
	}
	portTCP8080 = model.Service{
		PID:                procTestService1.pid,
		GeneratedName:      "test-service-1-generated",
		DDService:          "test-service-1",
		DDServiceInjected:  true,
		Ports:              []uint16{8080},
		APMInstrumentation: string(apm.None),
		RSS:                100 * 1024 * 1024,
		CommandLine:        []string{"test-service-1"},
		StartTimeSecs:      procLaunchedSeconds,
	}
	portTCP8080UpdatedRSS = model.Service{
		PID:                procTestService1.pid,
		GeneratedName:      "test-service-1-generated",
		DDService:          "test-service-1",
		DDServiceInjected:  true,
		Ports:              []uint16{8080},
		APMInstrumentation: string(apm.None),
		RSS:                200 * 1024 * 1024,
		CommandLine:        []string{"test-service-1"},
		StartTimeSecs:      procLaunchedSeconds,
	}
	portTCP8080DifferentPID = model.Service{
		PID:                procTestService1DifferentPID.pid,
		GeneratedName:      "test-service-1-generated",
		DDService:          "test-service-1",
		DDServiceInjected:  true,
		Ports:              []uint16{8080},
		APMInstrumentation: string(apm.Injected),
		CommandLine:        []string{"test-service-1"},
		StartTimeSecs:      procLaunchedSeconds,
	}
	portTCP8081 = model.Service{
		PID:           procIgnoreService1.pid,
		GeneratedName: "ignore-1",
		Ports:         []uint16{8081},
		StartTimeSecs: procLaunchedSeconds,
	}
	portTCP5000 = model.Service{
		PID:           procPythonService.pid,
		GeneratedName: "python-service",
		Language:      "python",
		Ports:         []uint16{5000},
		CommandLine:   pythonCommandLine,
		StartTimeSecs: procLaunchedSeconds,
	}
	portTCP5432 = model.Service{
		PID:           procTestService1Repeat.pid,
		GeneratedName: "test-service-1",
		Ports:         []uint16{5432},
		CommandLine:   []string{"test-service-1"},
		StartTimeSecs: procLaunchedSeconds,
	}
)

func calcTime(additionalTime time.Duration) time.Time {
	unix := time.Unix(int64(procLaunchedSeconds), 0)
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
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP5000,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(0),
				},
				{
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP5000,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(1 * time.Minute),
				},
				{
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP5000,
						portTCP8080UpdatedRSS,
						portTCP8081,
					}},
					time: calcTime(20 * time.Minute),
				},
				{
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
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1-generated",
						DDService:            "test-service-1",
						ServiceNameSource:    "injected",
						ServiceType:          "web_service",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(1 * time.Minute).Unix(),
						Ports:                []uint16{8080},
						PID:                  99,
						CommandLine:          []string{"test-service-1"},
						APMInstrumentation:   "none",
						RSSMemory:            100 * 1024 * 1024,
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1-generated",
						DDService:            "test-service-1",
						ServiceNameSource:    "injected",
						ServiceType:          "web_service",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(20 * time.Minute).Unix(),
						Ports:                []uint16{8080},
						PID:                  99,
						CommandLine:          []string{"test-service-1"},
						APMInstrumentation:   "none",
						RSSMemory:            200 * 1024 * 1024,
					},
				},
				{
					RequestType: "end-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1-generated",
						DDService:            "test-service-1",
						ServiceNameSource:    "injected",
						ServiceType:          "web_service",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(20 * time.Minute).Unix(),
						Ports:                []uint16{8080},
						PID:                  99,
						CommandLine:          []string{"test-service-1"},
						APMInstrumentation:   "none",
						RSSMemory:            200 * 1024 * 1024,
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:  "1",
						ServiceName:          "python-service",
						GeneratedServiceName: "python-service",
						ServiceType:          "web_service",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(1 * time.Minute).Unix(),
						Ports:                []uint16{5000},
						PID:                  500,
						ServiceLanguage:      "python",
						CommandLine:          pythonCommandLine,
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:  "1",
						ServiceName:          "python-service",
						GeneratedServiceName: "python-service",
						ServiceType:          "web_service",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(20 * time.Minute).Unix(),
						Ports:                []uint16{5000},
						PID:                  500,
						ServiceLanguage:      "python",
						CommandLine:          pythonCommandLine,
					},
				},
			},
		},
		{
			name: "repeated_service_name",
			checkRun: []*checkRun{
				{
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					}},
					time: calcTime(0),
				},
				{
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					}},
					time: calcTime(1 * time.Minute),
				},
				{
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					}},
					time: calcTime(20 * time.Minute),
				},
				{
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
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1",
						ServiceType:          "db",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(1 * time.Minute).Unix(),
						Ports:                []uint16{5432},
						PID:                  101,
						CommandLine:          []string{"test-service-1"},
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1-generated",
						DDService:            "test-service-1",
						ServiceNameSource:    "injected",
						ServiceType:          "web_service",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(1 * time.Minute).Unix(),
						Ports:                []uint16{8080},
						PID:                  99,
						CommandLine:          []string{"test-service-1"},
						APMInstrumentation:   "none",
						RSSMemory:            100 * 1024 * 1024,
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1",
						ServiceType:          "db",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(20 * time.Minute).Unix(),
						Ports:                []uint16{5432},
						PID:                  101,
						CommandLine:          []string{"test-service-1"},
					},
				},
				{
					RequestType: "end-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1",
						ServiceType:          "db",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(20 * time.Minute).Unix(),
						Ports:                []uint16{5432},
						PID:                  101,
						CommandLine:          []string{"test-service-1"},
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1-generated",
						DDService:            "test-service-1",
						ServiceNameSource:    "injected",
						ServiceType:          "web_service",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(20 * time.Minute).Unix(),
						Ports:                []uint16{8080},
						PID:                  99,
						CommandLine:          []string{"test-service-1"},
						APMInstrumentation:   "none",
						RSSMemory:            100 * 1024 * 1024,
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
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(0),
				},
				{
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(1 * time.Minute),
				},
				{
					servicesResp: &model.ServicesResponse{Services: []model.Service{
						portTCP22,
						portTCP8080DifferentPID,
					}},
					time: calcTime(21 * time.Minute),
				},
				{
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
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1-generated",
						DDService:            "test-service-1",
						ServiceNameSource:    "injected",
						ServiceType:          "web_service",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(1 * time.Minute).Unix(),
						Ports:                []uint16{8080},
						PID:                  99,
						CommandLine:          []string{"test-service-1"},
						APMInstrumentation:   "none",
						RSSMemory:            100 * 1024 * 1024,
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:  "1",
						ServiceName:          "test-service-1",
						GeneratedServiceName: "test-service-1-generated",
						DDService:            "test-service-1",
						ServiceNameSource:    "injected",
						ServiceType:          "web_service",
						HostName:             host,
						Env:                  "",
						StartTime:            calcTime(0).Unix(),
						LastSeen:             calcTime(22 * time.Minute).Unix(),
						Ports:                []uint16{8080},
						PID:                  102,
						CommandLine:          []string{"test-service-1"},
						APMInstrumentation:   "injected",
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

				_, mHostname := hostnameinterface.NewMock(hostnameinterface.MockHostname(host))

				mTimer := NewMocktimer(ctrl)
				mTimer.EXPECT().Now().Return(cr.time).AnyTimes()

				// set mocks
				check.os.(*linuxImpl).getSysProbeClient = func() (systemProbeClient, error) {
					return mSysProbe, nil
				}
				check.os.(*linuxImpl).time = mTimer
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
