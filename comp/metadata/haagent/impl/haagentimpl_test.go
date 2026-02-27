// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package haagentimpl

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

func getProvides(t *testing.T, confOverrides map[string]any) (Provides, error) {
	cfg := config.NewMock(t)
	for k, v := range confOverrides {
		cfg.SetWithoutSource(k, v)
	}
	r := Requires{
		Log:        logmock.New(t),
		Config:     cfg,
		Serializer: serializermock.NewMetricSerializer(t),
		HaAgent:    haagentmock.NewMockHaAgent(),
		Hostname:   hostnameimpl.NewHostnameService(),
	}
	return NewComponent(r)
}

func getTestInventoryPayload(t *testing.T, confOverrides map[string]any) *haagentimpl {
	p, _ := getProvides(t, confOverrides)
	return p.Comp.(*haagentimpl)
}

func TestGetPayload(t *testing.T) {
	overrides := map[string]any{}

	io := getTestInventoryPayload(t, overrides)
	io.hostname = "hostname-for-test"

	haAgentMock := io.haAgent.(haagentmock.Component)
	haAgentMock.SetEnabled(true)

	startTime := time.Now().UnixNano()

	p := io.getPayload()
	payload := p.(*Payload)

	data := &haAgentMetadata{
		Enabled: true,
		State:   "standby",
	}

	assert.True(t, payload.Timestamp >= startTime)
	assert.Equal(t, "hostname-for-test", payload.Hostname)
	assert.Equal(t, data, payload.Metadata)

	// check payload is a copy
	io.data.State = "active"
	assert.Equal(t, data, payload.Metadata)
}

func TestGet(t *testing.T) {
	overrides := map[string]any{}
	io := getTestInventoryPayload(t, overrides)
	haAgentMock := io.haAgent.(haagentmock.Component)
	haAgentMock.SetEnabled(true)

	// Collect metadata
	io.refreshMetadata()

	p := io.Get()

	// verify that the return struct is a copy
	p.State = ""
	assert.Equal(t, "standby", io.data.State)
	assert.NotEqual(t, p.State, io.data.State)
}

func TestFlareProviderFilename(t *testing.T) {
	io := getTestInventoryPayload(t, nil)
	assert.Equal(t, "ha-agent.json", io.FlareFileName)
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

			keys := maps.Keys(stats)

			assert.Contains(t, keys, "ha_agent_metadata")
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerStatusProvider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerStatusProvider.HTML(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}
