// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"cmp"
	"net/http"
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

const (
	dummyContainerID = "abcd"
)

var (
	bootTimeMilli     = uint64(time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC).UnixMilli())
	procLaunchedMilli = bootTimeMilli + uint64((12 * time.Hour).Milliseconds())
	pythonCommandLine = []string{"python", "-m", "foobar.main"}
)

var (
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
)

var (
	portTCP8080 = model.Service{
		PID:                        procTestService1.pid,
		Name:                       "test-service-1",
		GeneratedName:              "test-service-1-generated",
		GeneratedNameSource:        "test-service-1-generated-source",
		ContainerServiceName:       "test-service-1-container",
		ContainerServiceNameSource: "service",
		ContainerTags: []string{
			"service:test-service-1-container",
			"other:tag",
		},
		DDService:          "test-service-1",
		DDServiceInjected:  true,
		Ports:              []uint16{8080},
		APMInstrumentation: string(apm.None),
		Type:               "web_service",
		RSS:                100 * 1024 * 1024,
		CPUCores:           1.5,
		CommandLine:        []string{"test-service-1"},
		StartTimeMilli:     procLaunchedMilli,
		ContainerID:        dummyContainerID,
		RxBytes:            100,
		TxBytes:            200,
		RxBps:              10,
		TxBps:              20,
	}
	portTCP8080UpdatedRSS = model.Service{
		PID:                        procTestService1.pid,
		Name:                       "test-service-1",
		GeneratedName:              "test-service-1-generated",
		GeneratedNameSource:        "test-service-1-generated-source",
		ContainerServiceName:       "test-service-1-container",
		ContainerServiceNameSource: "service",
		ContainerTags: []string{
			"service:test-service-1-container",
			"other:tag",
		},
		DDService:          "test-service-1",
		DDServiceInjected:  true,
		Ports:              []uint16{8080},
		APMInstrumentation: string(apm.None),
		Type:               "web_service",
		RSS:                200 * 1024 * 1024,
		CPUCores:           1.5,
		CommandLine:        []string{"test-service-1"},
		StartTimeMilli:     procLaunchedMilli,
		ContainerID:        dummyContainerID,
		RxBytes:            1000,
		TxBytes:            2000,
		RxBps:              900,
		TxBps:              800,
	}
	portTCP5000 = model.Service{
		PID:                        procPythonService.pid,
		Name:                       "python-service",
		GeneratedName:              "python-service",
		GeneratedNameSource:        "python-service-source",
		AdditionalGeneratedNames:   []string{"bar", "foo"},
		ContainerServiceName:       "test-service-1-container",
		ContainerServiceNameSource: "app",
		ContainerTags: []string{
			"app:test-service-1-app",
			"other:tag",
		},
		Language:       "python",
		Ports:          []uint16{5000},
		Type:           "web_service",
		CommandLine:    pythonCommandLine,
		StartTimeMilli: procLaunchedMilli,
		ContainerID:    dummyContainerID,
	}
)

func calcTime(additionalTime time.Duration) time.Time {
	unix := time.UnixMilli(int64(procLaunchedMilli))
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
					servicesResp: &model.ServicesResponse{StartedServices: []model.Service{
						portTCP5000,
						portTCP8080,
					}},
					time: calcTime(0),
				},
				{
					servicesResp: &model.ServicesResponse{HeartbeatServices: []model.Service{
						portTCP5000,
						portTCP8080UpdatedRSS,
					}},
					time: calcTime(20 * time.Minute),
				},
				{
					servicesResp: &model.ServicesResponse{StoppedServices: []model.Service{
						portTCP8080UpdatedRSS,
					}},
					time: calcTime(20 * time.Minute),
				},
			},
			wantEvents: []*event{
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:        "1",
						ServiceName:                "test-service-1",
						GeneratedServiceName:       "test-service-1-generated",
						GeneratedServiceNameSource: "test-service-1-generated-source",
						ContainerServiceName:       "test-service-1-container",
						ContainerServiceNameSource: "service",
						ContainerTags: []string{
							"service:test-service-1-container",
							"other:tag",
						},
						DDService:          "test-service-1",
						ServiceNameSource:  "injected",
						ServiceType:        "web_service",
						HostName:           host,
						Env:                "",
						StartTime:          calcTime(0).Unix(),
						StartTimeMilli:     calcTime(0).UnixMilli(),
						LastSeen:           calcTime(0).Unix(),
						Ports:              []uint16{8080},
						PID:                99,
						CommandLine:        []string{"test-service-1"},
						APMInstrumentation: "none",
						RSSMemory:          100 * 1024 * 1024,
						CPUCores:           1.5,
						ContainerID:        dummyContainerID,
						RxBytes:            100,
						TxBytes:            200,
						RxBps:              10,
						TxBps:              20,
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:        "1",
						ServiceName:                "test-service-1",
						GeneratedServiceName:       "test-service-1-generated",
						GeneratedServiceNameSource: "test-service-1-generated-source",
						ContainerServiceName:       "test-service-1-container",
						ContainerServiceNameSource: "service",
						ContainerTags: []string{
							"service:test-service-1-container",
							"other:tag",
						},
						DDService:          "test-service-1",
						ServiceNameSource:  "injected",
						ServiceType:        "web_service",
						HostName:           host,
						Env:                "",
						StartTime:          calcTime(0).Unix(),
						StartTimeMilli:     calcTime(0).UnixMilli(),
						LastSeen:           calcTime(20 * time.Minute).Unix(),
						Ports:              []uint16{8080},
						PID:                99,
						CommandLine:        []string{"test-service-1"},
						APMInstrumentation: "none",
						RSSMemory:          200 * 1024 * 1024,
						CPUCores:           1.5,
						ContainerID:        dummyContainerID,
						RxBytes:            1000,
						TxBytes:            2000,
						RxBps:              900,
						TxBps:              800,
					},
				},
				{
					RequestType: "end-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:        "1",
						ServiceName:                "test-service-1",
						GeneratedServiceName:       "test-service-1-generated",
						GeneratedServiceNameSource: "test-service-1-generated-source",
						ContainerServiceName:       "test-service-1-container",
						ContainerServiceNameSource: "service",
						ContainerTags: []string{
							"service:test-service-1-container",
							"other:tag",
						},
						DDService:          "test-service-1",
						ServiceNameSource:  "injected",
						ServiceType:        "web_service",
						HostName:           host,
						Env:                "",
						StartTime:          calcTime(0).Unix(),
						StartTimeMilli:     calcTime(0).UnixMilli(),
						LastSeen:           calcTime(20 * time.Minute).Unix(),
						Ports:              []uint16{8080},
						PID:                99,
						CommandLine:        []string{"test-service-1"},
						APMInstrumentation: "none",
						RSSMemory:          200 * 1024 * 1024,
						CPUCores:           1.5,
						ContainerID:        dummyContainerID,
						RxBytes:            1000,
						TxBytes:            2000,
						RxBps:              900,
						TxBps:              800,
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:        "1",
						ServiceName:                "python-service",
						GeneratedServiceName:       "python-service",
						GeneratedServiceNameSource: "python-service-source",
						AdditionalGeneratedNames:   []string{"bar", "foo"},
						ContainerServiceName:       "test-service-1-container",
						ContainerServiceNameSource: "app",
						ContainerTags: []string{
							"app:test-service-1-app",
							"other:tag",
						},
						ServiceType:     "web_service",
						HostName:        host,
						Env:             "",
						StartTime:       calcTime(0).Unix(),
						StartTimeMilli:  calcTime(0).UnixMilli(),
						LastSeen:        calcTime(0).Unix(),
						Ports:           []uint16{5000},
						PID:             500,
						ServiceLanguage: "python",
						CommandLine:     pythonCommandLine,
						ContainerID:     dummyContainerID,
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion:        "1",
						ServiceName:                "python-service",
						GeneratedServiceName:       "python-service",
						GeneratedServiceNameSource: "python-service-source",
						AdditionalGeneratedNames:   []string{"bar", "foo"},
						ContainerServiceName:       "test-service-1-container",
						ContainerServiceNameSource: "app",
						ContainerTags: []string{
							"app:test-service-1-app",
							"other:tag",
						},
						ServiceType:     "web_service",
						HostName:        host,
						Env:             "",
						StartTime:       calcTime(0).Unix(),
						StartTimeMilli:  calcTime(0).UnixMilli(),
						LastSeen:        calcTime(20 * time.Minute).Unix(),
						Ports:           []uint16{5000},
						PID:             500,
						ServiceLanguage: "python",
						CommandLine:     pythonCommandLine,
						ContainerID:     dummyContainerID,
					},
				},
			},
		},
	}

	makeServiceResponseWithTime := func(responseTime time.Time, resp *model.ServicesResponse) *model.ServicesResponse {
		respWithTime := &model.ServicesResponse{
			StartedServices:   make([]model.Service, 0, len(resp.StartedServices)),
			StoppedServices:   make([]model.Service, 0, len(resp.StoppedServices)),
			HeartbeatServices: make([]model.Service, 0, len(resp.HeartbeatServices)),
		}

		for _, service := range resp.StartedServices {
			service.LastHeartbeat = responseTime.Unix()
			respWithTime.StartedServices = append(respWithTime.StartedServices, service)
		}
		for _, service := range resp.StoppedServices {
			service.LastHeartbeat = responseTime.Unix()
			respWithTime.StoppedServices = append(respWithTime.StoppedServices, service)
		}
		for _, service := range resp.HeartbeatServices {
			service.LastHeartbeat = responseTime.Unix()
			respWithTime.HeartbeatServices = append(respWithTime.HeartbeatServices, service)
		}

		return respWithTime
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// check and mocks setup
			check := newCheck()

			mSender := mocksender.NewMockSender(check.ID())
			mSender.SetupAcceptAll()

			err := check.Configure(
				mSender.GetSenderManager(),
				integration.FakeConfigHash,
				integration.Data{},
				nil,
				"test",
			)
			require.NoError(t, err)
			require.NotNil(t, check.os)

			for _, cr := range tc.checkRun {
				_, mHostname := hostnameinterface.NewMock(hostnameinterface.MockHostname(host))

				mTimer := NewMocktimer(ctrl)
				mTimer.EXPECT().Now().Return(cr.time).AnyTimes()

				// set mocks
				check.os.(*linuxImpl).getDiscoveryServices = func(_ *http.Client) (*model.ServicesResponse, error) {
					return makeServiceResponseWithTime(cr.time, cr.servicesResp), nil
				}
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
