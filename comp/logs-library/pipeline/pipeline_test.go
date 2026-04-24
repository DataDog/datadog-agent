// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// encodeAndGetTags runs the encoder on a minimal rendered message and returns
// the ddtags field of the resulting JSON payload. Used to detect whether a
// returned encoder has a host-tag provider wired.
func encodeAndGetTags(t *testing.T, encoder processor.Encoder) string {
	t.Helper()
	logsConfig := &config.LogsConfig{
		Service: "Service",
		Source:  "Source",
	}
	source := sources.NewLogSource("", logsConfig)
	msg := message.NewMessageWithSource([]byte("hi"), message.StatusInfo, source, 0)
	msg.State = message.StateRendered
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a"})

	require.NoError(t, encoder.Encode(msg, "unknown"))
	var payload struct {
		Tags string `json:"ddtags"`
	}
	require.NoError(t, json.Unmarshal(msg.GetContent(), &payload))
	return payload.Tags
}

func httpEndpoints() *config.Endpoints {
	return config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": true,
	})
}

// TestSelectEncoderHTTPNoProvider verifies that the HTTP/JSON path returns the
// shared JSONEncoder singleton (no host tags injected) when no host-tag
// provider is supplied, regardless of the send_host_tags toggle.
func TestSelectEncoderHTTPNoProvider(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.send_host_tags", true)

	encoder := selectEncoder(httpEndpoints(), sender.NewServerlessMeta(false), cfg, nil)

	// Identity check: should be the shared singleton, not a freshly-constructed encoder.
	assert.Same(t, processor.JSONEncoder, encoder)
	assert.Equal(t, "a", encodeAndGetTags(t, encoder))
}

// TestSelectEncoderHTTPProviderToggleDisabled verifies that the HTTP/JSON path
// falls back to the shared JSONEncoder singleton when the OPW send_host_tags
// toggle is false, even if a provider is supplied.
func TestSelectEncoderHTTPProviderToggleDisabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.send_host_tags", false)

	encoder := selectEncoder(
		httpEndpoints(),
		sender.NewServerlessMeta(false),
		cfg,
		func() []string { return []string{"host:x"} },
	)

	assert.Same(t, processor.JSONEncoder, encoder)
	assert.Equal(t, "a", encodeAndGetTags(t, encoder))
}

// TestSelectEncoderHTTPProviderToggleEnabled verifies that the HTTP/JSON path
// wires the host-tag provider into a new JSON encoder when the OPW
// send_host_tags toggle is true.
func TestSelectEncoderHTTPProviderToggleEnabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.send_host_tags", true)

	encoder := selectEncoder(
		httpEndpoints(),
		sender.NewServerlessMeta(false),
		cfg,
		func() []string { return []string{"host:x", "env:prod"} },
	)

	// Identity check: should not be the shared singleton.
	assert.NotSame(t, processor.JSONEncoder, encoder)
	assert.Equal(t, "a,host:x,env:prod", encodeAndGetTags(t, encoder))
}

// TestSelectEncoderNonHTTPIgnoresProvider verifies that non-HTTP pipelines
// (proto, raw) keep their original encoders even when a host-tag provider is
// supplied and the OPW send_host_tags toggle is true. Host tag injection is
// only applicable to the HTTP/JSON path used by OPW.
func TestSelectEncoderNonHTTPIgnoresProvider(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.send_host_tags", true)

	provider := func() []string { return []string{"host:x"} }

	protoEndpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": false,
	})
	protoEndpoints.UseProto = true
	assert.Same(t, processor.ProtoEncoder, selectEncoder(protoEndpoints, sender.NewServerlessMeta(false), cfg, provider))

	rawEndpoints := config.NewMockEndpointsWithOptions([]config.Endpoint{config.NewMockEndpoint()}, map[string]interface{}{
		"use_http": false,
	})
	assert.Same(t, processor.RawEncoder, selectEncoder(rawEndpoints, sender.NewServerlessMeta(false), cfg, provider))
}

// TestSelectEncoderServerlessIgnoresProvider verifies that serverless mode
// always uses the serverless JSON encoder and ignores any host-tag provider.
func TestSelectEncoderServerlessIgnoresProvider(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.enabled", true)
	cfg.SetWithoutSource("observability_pipelines_worker.logs.send_host_tags", true)

	encoder := selectEncoder(
		httpEndpoints(),
		sender.NewServerlessMeta(true),
		cfg,
		func() []string { return []string{"host:x"} },
	)
	assert.Same(t, processor.JSONServerlessInitEncoder, encoder)
}
