// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentConfig(t *testing.T) {
	fileName := "testdata/config.yaml"
	c, err := NewConfigComponent(context.Background(), []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.eu", c.Get("site"))
	assert.Equal(t, "debug", c.Get("log_level"))
	assert.Equal(t, "https://trace.agent.datadoghq.eu", c.Get("apm_config.apm_dd_url"))
	assert.Equal(t, map[string]string{"io.opentelemetry.javaagent.spring.client": "spring.client"}, c.Get("otlp_config.traces.span_name_remappings"))
	assert.Equal(t, []string{"(GET|POST) /healthcheck"}, c.Get("apm_config.ignore_resources"))
	assert.Equal(t, 0, c.Get("apm_config.receiver_port"))
	assert.Equal(t, 10, c.Get("apm_config.trace_buffer"))
	assert.Equal(t, false, c.Get("otlp_config.traces.span_name_as_resource_name"))
	assert.Equal(t, nil, c.Get("apm_config.features"))
}

func TestAgentConfigDefaults(t *testing.T) {
	fileName := "testdata/config_default.yaml"
	c, err := NewConfigComponent(context.Background(), []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "https://trace.agent.datadoghq.com", c.Get("apm_config.apm_dd_url"))
	assert.Equal(t, 0, c.Get("apm_config.receiver_port"))
	assert.Equal(t, true, c.Get("otlp_config.traces.span_name_as_resource_name"))
	assert.Equal(t, []string{"enable_otlp_compute_top_level_by_span_kind"}, c.Get("apm_config.features"))
}

func TestNoDDExporter(t *testing.T) {
	fileName := "testdata/config_no_dd_exporter.yaml"
	_, err := NewConfigComponent(context.Background(), []string{fileName})
	assert.EqualError(t, err, "no datadog exporter found")
}

func TestMultipleDDExporters(t *testing.T) {
	fileName := "testdata/config_multiple_dd_exporters.yaml"
	_, err := NewConfigComponent(context.Background(), []string{fileName})
	assert.EqualError(t, err, "multiple datadog exporters found")
}
