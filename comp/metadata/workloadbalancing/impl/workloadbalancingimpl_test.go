// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"bytes"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadbalancing "github.com/DataDog/datadog-agent/comp/workloadbalancing/def"
	workloadbalancingmock "github.com/DataDog/datadog-agent/comp/workloadbalancing/mock"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

func getProvides(t *testing.T, confOverrides map[string]any) (Provides, error) {
	cfg := config.NewMock(t)
	for k, v := range confOverrides {
		cfg.SetInTest(k, v)
	}
	r := Requires{
		Log:               logmock.New(t),
		Config:            cfg,
		Serializer:        serializermock.NewMetricSerializer(t),
		WorkloadBalancing: workloadbalancingmock.NewMock(),
		Hostname:          hostnameimpl.NewHostnameService(),
	}
	return NewComponent(r)
}

func getTestInventoryPayload(t *testing.T, confOverrides map[string]any) *workloadbalancingimpl {
	p, _ := getProvides(t, confOverrides)
	return p.Comp.(*workloadbalancingimpl)
}

func enabledMock(io *workloadbalancingimpl) workloadbalancingmock.Component {
	mock := io.workloadBalancing.(workloadbalancingmock.Component)
	mock.SetEnabled(true)
	return mock
}

func TestGetPayload(t *testing.T) {
	io := getTestInventoryPayload(t, nil)
	io.hostname = "hostname-for-test"

	mock := enabledMock(io)
	mock.SetGroupState("group-a", workloadbalancing.Active)
	mock.SetGroupState("group-b", workloadbalancing.Standby)

	startTime := time.Now().UnixNano()

	payload := io.getPayload().(*Payload)

	data := &workloadBalancingMetadata{
		Enabled: true,
		Groups: map[string]string{
			"group-a": string(workloadbalancing.Active),
			"group-b": string(workloadbalancing.Standby),
		},
	}

	assert.True(t, payload.Timestamp >= startTime)
	assert.Equal(t, "hostname-for-test", payload.Hostname)
	assert.Equal(t, data, payload.Metadata)

	// check payload is a copy, including the group map
	io.data.Groups["group-a"] = string(workloadbalancing.Standby)
	assert.Equal(t, data, payload.Metadata)
}

func TestGetPayloadDisabled(t *testing.T) {
	io := getTestInventoryPayload(t, nil)

	payload := io.getPayload().(*Payload)

	assert.Nil(t, payload.Metadata)
}

func TestGet(t *testing.T) {
	io := getTestInventoryPayload(t, nil)
	mock := enabledMock(io)
	mock.SetGroupState("group-a", workloadbalancing.Active)

	io.refreshMetadata()

	p := io.Get()

	// verify that the return struct is a copy
	p.Groups["group-a"] = ""
	assert.Equal(t, string(workloadbalancing.Active), io.data.Groups["group-a"])
}

func TestGetReflectsLiveStateWithoutExplicitRefresh(t *testing.T) {
	io := getTestInventoryPayload(t, nil)
	mock := enabledMock(io)

	io.refreshMetadata()
	assert.NotNil(t, io.Get(), "metadata should be present while workload balancing is enabled")

	// Simulate the state changing after the periodic inventory collector last ran, without a new
	// collection cycle (or an explicit refreshMetadata call) happening in between.
	mock.SetEnabled(false)

	assert.Nil(t, io.Get(), "Get() must reflect the current state, not the value cached by the last periodic collection")
}

func TestFlareProviderFilename(t *testing.T) {
	io := getTestInventoryPayload(t, nil)
	assert.Equal(t, "workload-balancing.json", io.FlareFileName)
}

func TestStatusHeaderProvider(t *testing.T) {
	ret, _ := getProvides(t, nil)

	headerStatusProvider := ret.StatusHeaderProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerStatusProvider.JSON(false, stats)

			keys := slices.Collect(maps.Keys(stats))

			assert.Contains(t, keys, "workload_balancing_metadata")
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerStatusProvider.Text(false, b)

			assert.NoError(t, err)
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerStatusProvider.HTML(false, b)

			assert.NoError(t, err)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestStatusHeaderProviderEnabled(t *testing.T) {
	io := getTestInventoryPayload(t, nil)
	mock := enabledMock(io)
	mock.SetGroupState("group-a", workloadbalancing.Active)
	io.refreshMetadata()

	t.Run("Text", func(t *testing.T) {
		b := new(bytes.Buffer)
		err := io.Text(false, b)

		assert.NoError(t, err)
		assert.Contains(t, b.String(), "enabled: true")
		assert.Contains(t, b.String(), "group-a: active")
	})

	t.Run("HTML", func(t *testing.T) {
		b := new(bytes.Buffer)
		err := io.HTML(false, b)

		assert.NoError(t, err)
		assert.Contains(t, b.String(), "enabled: true")
		assert.Contains(t, b.String(), "group-a: active")
	})
}
