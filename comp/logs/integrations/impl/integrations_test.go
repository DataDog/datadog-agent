// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package integrationsimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewComponent(t *testing.T) {
	comp := NewLogsIntegration()

	assert.NotNil(t, comp, "Integrations component nil.")
}

// TestSendandSubscribe tests sending a log through the integrations component.
func TestSendandSubscribe(t *testing.T) {
	comp := NewLogsIntegration()

	go func() {
		comp.SendLog("test log", "integration1")
	}()

	log := <-comp.Subscribe()
	assert.Equal(t, "test log", log.Log)
	assert.Equal(t, "integration1", log.IntegrationID)
}
