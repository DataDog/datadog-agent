// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func mockSenderEvents(t *testing.T, m *mocksender.MockSender) []*event {
	t.Helper()

	var gotEvents []*event
	for _, call := range m.Calls {
		evType := call.Arguments.Get(1).(string)
		assert.Equal(t, "service-discovery", evType)

		raw := call.Arguments.Get(0).([]byte)
		var ev *event
		err := json.Unmarshal(raw, &ev)
		require.NoError(t, err)
		gotEvents = append(gotEvents, ev)
	}
	return gotEvents
}

func Test_telemetrySender(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mSender := mocksender.NewMockSender("test-servicediscovery")
	mSender.SetupAcceptAll()

	mTimer := NewMocktimer(ctrl)
	now := time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC)
	mTimer.EXPECT().Now().Return(now).AnyTimes()

	host := "test-host"
	_, mHostname := hostnameinterface.NewMock(hostnameinterface.MockHostname(host))

	ts := newTelemetrySender(mSender)
	ts.hostname = mHostname
	ts.time = mTimer

	svc := serviceInfo{
		process: processInfo{
			PID:     0,
			CmdLine: nil,
			Env:     nil,
			Cwd:     "",
			Stat: procStat{
				StartTime: uint64(now.Add(-20 * time.Minute).Unix()),
			},
			Ports: nil,
		},
		meta: serviceMetadata{
			Name:               "test-service",
			Language:           "jvm",
			Type:               "web_service",
			APMInstrumentation: "injected",
		},
		LastHeartbeat: now,
	}

	ts.sendStartServiceEvent(svc)
	ts.sendHeartbeatServiceEvent(svc)
	ts.sendEndServiceEvent(svc)

	wantEvents := []*event{
		{
			RequestType: "start-service",
			APIVersion:  "v2",
			Payload: &eventPayload{
				NamingSchemaVersion: "1",
				ServiceName:         "test-service",
				HostName:            "test-host",
				Env:                 "",
				ServiceLanguage:     "jvm",
				ServiceType:         "web_service",
				StartTime:           1715557200,
				LastSeen:            1715558400,
				APMInstrumentation:  "injected",
				ServiceNameSource:   "generated",
			},
		},
		{
			RequestType: "heartbeat-service",
			APIVersion:  "v2",
			Payload: &eventPayload{
				NamingSchemaVersion: "1",
				ServiceName:         "test-service",
				HostName:            "test-host",
				Env:                 "",
				ServiceLanguage:     "jvm",
				ServiceType:         "web_service",
				StartTime:           1715557200,
				LastSeen:            1715558400,
				APMInstrumentation:  "injected",
				ServiceNameSource:   "generated",
			},
		},
		{
			RequestType: "end-service",
			APIVersion:  "v2",
			Payload: &eventPayload{
				NamingSchemaVersion: "1",
				ServiceName:         "test-service",
				HostName:            "test-host",
				Env:                 "",
				ServiceLanguage:     "jvm",
				ServiceType:         "web_service",
				StartTime:           1715557200,
				LastSeen:            1715558400,
				APMInstrumentation:  "injected",
				ServiceNameSource:   "generated",
			},
		},
	}

	mSender.AssertNumberOfCalls(t, "EventPlatformEvent", 3)
	gotEvents := mockSenderEvents(t, mSender)
	if diff := cmp.Diff(wantEvents, gotEvents); diff != "" {
		t.Errorf("event platform events mismatch (-want +got):\n%s", diff)
	}
}

func Test_telemetrySender_name_provided(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mSender := mocksender.NewMockSender("test-servicediscovery")
	mSender.SetupAcceptAll()

	mTimer := NewMocktimer(ctrl)
	now := time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC)
	mTimer.EXPECT().Now().Return(now).AnyTimes()

	host := "test-host"
	_, mHostname := hostnameinterface.NewMock(hostnameinterface.MockHostname(host))

	ts := newTelemetrySender(mSender)
	ts.hostname = mHostname
	ts.time = mTimer

	svc := serviceInfo{
		process: processInfo{
			PID:     0,
			CmdLine: nil,
			Env:     nil,
			Cwd:     "",
			Stat: procStat{
				StartTime: uint64(now.Add(-20 * time.Minute).Unix()),
			},
			Ports: nil,
		},
		meta: serviceMetadata{
			Name:               "test-service",
			Language:           "jvm",
			Type:               "web_service",
			APMInstrumentation: "injected",
			FromDDService:      true,
		},
		LastHeartbeat: now,
	}

	ts.sendStartServiceEvent(svc)
	ts.sendHeartbeatServiceEvent(svc)
	ts.sendEndServiceEvent(svc)

	wantEvents := []*event{
		{
			RequestType: "start-service",
			APIVersion:  "v2",
			Payload: &eventPayload{
				NamingSchemaVersion: "1",
				ServiceName:         "test-service",
				HostName:            "test-host",
				Env:                 "",
				ServiceLanguage:     "jvm",
				ServiceType:         "web_service",
				StartTime:           1715557200,
				LastSeen:            1715558400,
				APMInstrumentation:  "injected",
				ServiceNameSource:   "provided",
			},
		},
		{
			RequestType: "heartbeat-service",
			APIVersion:  "v2",
			Payload: &eventPayload{
				NamingSchemaVersion: "1",
				ServiceName:         "test-service",
				HostName:            "test-host",
				Env:                 "",
				ServiceLanguage:     "jvm",
				ServiceType:         "web_service",
				StartTime:           1715557200,
				LastSeen:            1715558400,
				APMInstrumentation:  "injected",
				ServiceNameSource:   "provided",
			},
		},
		{
			RequestType: "end-service",
			APIVersion:  "v2",
			Payload: &eventPayload{
				NamingSchemaVersion: "1",
				ServiceName:         "test-service",
				HostName:            "test-host",
				Env:                 "",
				ServiceLanguage:     "jvm",
				ServiceType:         "web_service",
				StartTime:           1715557200,
				LastSeen:            1715558400,
				APMInstrumentation:  "injected",
				ServiceNameSource:   "provided",
			},
		},
	}

	mSender.AssertNumberOfCalls(t, "EventPlatformEvent", 3)
	gotEvents := mockSenderEvents(t, mSender)
	if diff := cmp.Diff(wantEvents, gotEvents); diff != "" {
		t.Errorf("event platform events mismatch (-want +got):\n%s", diff)
	}
}
