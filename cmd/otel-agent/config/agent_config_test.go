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
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "debug", c.Get("log_level"))
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
