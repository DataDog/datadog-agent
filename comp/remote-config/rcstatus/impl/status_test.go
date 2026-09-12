// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package rcstatusimpl

import (
	"bytes"
	"expvar"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusOuput(t *testing.T) {
	provides := NewComponent(Requires{
		Config: config.NewMock(t),
	})

	statusProvider := provides.StatusProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			statusProvider.JSON(false, stats)

			assert.NotEmpty(t, stats)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := statusProvider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := statusProvider.HTML(false, b)

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

// TestStatusOutputAdditionalInstances checks that extra Remote Configuration
// clients (e.g. cluster_agent.remote_configuration.additional_clients) are
// rendered, and that the default client is not listed twice.
func TestStatusOutputAdditionalInstances(t *testing.T) {
	// The provider reads the process-wide "remoteConfigStatus" expvar, published
	// by pkg/config/remote/service. That package is not linked into this test
	// binary, so publish an equivalent map with the same shape.
	const varName = "remoteConfigStatus"
	var rcStatus *expvar.Map
	switch existing := expvar.Get(varName).(type) {
	case nil:
		rcStatus = expvar.NewMap(varName)
	case *expvar.Map:
		rcStatus = existing
	default:
		t.Fatalf("%s is published as %T, expected *expvar.Map", varName, existing)
	}
	for name, value := range map[string]string{"orgEnabled": "true", "apiKeyScoped": "true", "lastError": ""} {
		v := &expvar.String{}
		v.Set(value)
		rcStatus.Set(name, v)
	}

	instances := &expvar.Map{}
	instances.Init()
	for name, authorized := range map[string]string{
		"Remote Config": "true",
		"autoscaling":   "true",
		"broken":        "false",
	} {
		entry := &expvar.Map{}
		entry.Init()
		orgEnabled := &expvar.String{}
		orgEnabled.Set("true")
		keyAuthorized := &expvar.String{}
		keyAuthorized.Set(authorized)
		lastError := &expvar.String{}
		endpoint := &expvar.String{}
		endpoint.Set("https://config." + name + ".example.com")
		entry.Set("orgEnabled", orgEnabled)
		entry.Set("apiKeyScoped", keyAuthorized)
		entry.Set("lastError", lastError)
		entry.Set("endpoint", endpoint)
		instances.Set(name, entry)
	}
	rcStatus.Set("instances", instances)
	t.Cleanup(func() { rcStatus.Set("instances", &expvar.Map{}) })

	provides := NewComponent(Requires{Config: config.NewMock(t)})
	statusProvider := provides.StatusProvider.Provider

	b := new(bytes.Buffer)
	require.NoError(t, statusProvider.Text(false, b))
	out := b.String()

	assert.Contains(t, out, "- autoscaling")
	assert.Contains(t, out, "- broken")
	assert.Contains(t, out, "Not authorized", "an unauthorized extra client must be visible")
	// The endpoint is the point of extra clients: they can target another backend.
	assert.Contains(t, out, "Endpoint: https://config.autoscaling.example.com")
	// The default client is described by the top-level fields; it must not also
	// appear in the additional-client list.
	assert.NotContains(t, out, "- Remote Config")

	h := new(bytes.Buffer)
	require.NoError(t, statusProvider.HTML(false, h))
	assert.Contains(t, h.String(), "autoscaling")
	assert.NotContains(t, h.String(), "<li>Remote Config")

	// With only the default client configured -- the overwhelmingly common case --
	// the output must not grow an empty section.
	t.Run("default client only", func(t *testing.T) {
		onlyDefault := &expvar.Map{}
		onlyDefault.Init()
		entry := &expvar.Map{}
		entry.Init()
		orgEnabled := &expvar.String{}
		orgEnabled.Set("true")
		entry.Set("orgEnabled", orgEnabled)
		onlyDefault.Set("Remote Config", entry)
		rcStatus.Set("instances", onlyDefault)

		b := new(bytes.Buffer)
		require.NoError(t, statusProvider.Text(false, b))
		assert.NotContains(t, b.String(), "Additional clients")
		assert.Contains(t, b.String(), "Organization enabled: True")
	})
}
