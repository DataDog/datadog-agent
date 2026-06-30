// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package fx

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestNoopIntegrationsSendLog(t *testing.T) {
	comp := NewNoopComponent()

	// Capture stderr so we can assert the output; stdout must remain clean for --json mode.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	comp.SendLog("hello from check", "my_check:abc123")

	w.Close()
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	os.Stderr = oldStderr

	assert.True(t, strings.Contains(string(out), "hello from check"), "output should contain the log message")
	assert.True(t, strings.Contains(string(out), "my_check:abc123"), "output should contain the integration ID")
}

func TestNoopIntegrationsChannelsAreNil(t *testing.T) {
	comp := NewNoopComponent()
	assert.Nil(t, comp.Subscribe(), "Subscribe channel should be nil")
	assert.Nil(t, comp.SubscribeIntegration(), "SubscribeIntegration channel should be nil")
}

func TestNoopIntegrationsRegisterIntegrationIsNoop(_ *testing.T) {
	comp := NewNoopComponent()
	// Should not panic
	comp.RegisterIntegration("my-id", integration.Config{Name: "my_check"})
}
