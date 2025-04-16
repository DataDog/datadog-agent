// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package integrationsimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
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

// TestReceiveEmptyConfig ensures that ReceiveIntegration doesn't send an empty
// configuration to subscribers
func TestReceiveEmptyConfig(t *testing.T) {
	logsIntegration := NewLogsIntegration()
	integrationChan := logsIntegration.SubscribeIntegration()

	mockConf := &integration.Config{}
	mockConf.Provider = "container"
	mockConf.LogsConfig = integration.Data(``)

	go func() {
		logsIntegration.RegisterIntegration("12345", *mockConf)
	}()

	select {
	case msg := <-integrationChan:
		assert.Fail(t, "Expected channel to not receive logs, instead got:", msg)
	case <-time.After(100 * time.Millisecond):
		assert.True(t, true, "Channel remained empty.")
	}
}
