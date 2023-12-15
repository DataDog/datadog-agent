// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package internaltelemetry full description in README.md
package internaltelemetry

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func testCfg(serverURL string) *config.AgentConfig {
	cfg := config.New()
	cfg.TelemetryConfig.Endpoints[0].Host = serverURL
	cfg.TelemetryConfig.Enabled = true
	return cfg
}

func TestCrashParser(t *testing.T) {
	cfg := testCfg("http://dummy")
	lts := NewLogTelemetrySender(cfg, "testsvc", "go")
	assert.NotNil(t, lts)

	ts, ok := lts.(*logTelemetrySender)
	assert.True(t, ok)
	le := ts.formatMessage("Error", "This is a test crash message")
	assert.NotNil(t, le)
	body, err := json.MarshalIndent(le, "", "  ")
	assert.NoError(t, err)
	fmt.Printf("%v", string(body))
}
