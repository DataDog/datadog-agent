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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
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

	svc := serviceInfo{
		process: processInfo{
			Stat: procStat{
				StartTime: uint64(now.Add(-20 * time.Minute).Unix()),
				RSS:       500 * 1024 * 1024,
			},
		},
		service: model.Service{
			PID:         99,
			CommandLine: []string{"test-service", "--args"},
			Ports:       []uint16{80, 8080},
		},
		meta: ServiceMetadata{
			Name:               "test-service",
			Language:           "jvm",
			Type:               "web_service",
			APMInstrumentation: "injected",
			NameSource:         "generated",
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
				Ports:               []uint16{80, 8080},
				PID:                 99,
				CommandLine:         []string{"test-service", "--args"},
				RSSMemory:           500 * 1024 * 1024,
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
				Ports:               []uint16{80, 8080},
				PID:                 99,
				CommandLine:         []string{"test-service", "--args"},
				RSSMemory:           500 * 1024 * 1024,
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
				Ports:               []uint16{80, 8080},
				PID:                 99,
				CommandLine:         []string{"test-service", "--args"},
				RSSMemory:           500 * 1024 * 1024,
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

	svc := serviceInfo{
		process: processInfo{
			Stat: procStat{
				StartTime: uint64(now.Add(-20 * time.Minute).Unix()),
			},
		},
		service: model.Service{
			PID:         55,
			CommandLine: []string{"foo", "--option"},
		},
		meta: ServiceMetadata{
			Name:               "test-service",
			Language:           "jvm",
			Type:               "web_service",
			APMInstrumentation: "injected",
			NameSource:         "provided",
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
				PID:                 55,
				CommandLine:         []string{"foo", "--option"},
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
				PID:                 55,
				CommandLine:         []string{"foo", "--option"},
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
				PID:                 55,
				CommandLine:         []string{"foo", "--option"},
			},
		},
	}

	mSender.AssertNumberOfCalls(t, "EventPlatformEvent", 3)
	gotEvents := mockSenderEvents(t, mSender)
	if diff := cmp.Diff(wantEvents, gotEvents); diff != "" {
		t.Errorf("event platform events mismatch (-want +got):\n%s", diff)
	}
}
