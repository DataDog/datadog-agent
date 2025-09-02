// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !serverless

package integrationsimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

var defaultConfig = integration.Config{
	Provider:   "container",
	LogsConfig: integration.Data(`[{"type": "integration", "source": "foo", "service": "bar"}]`),
}

func TestNewComponent(t *testing.T) {
	comp := NewLogsIntegration(logmock.New(t), configmock.New(t))

	assert.NotNil(t, comp, "Integrations component nil.")
}

// TestSendandSubscribe tests sending a log through the integrations component.
func TestSendandSubscribe(t *testing.T) {
	comp := NewLogsIntegration(logmock.New(t), configmock.New(t))
	callbackCount := 0
	comp.SetActionCallback(func() error {
		callbackCount++
		return nil
	})

	go func() {
		comp.RegisterIntegration("integration1", defaultConfig)
		comp.SendLog("test log", "integration1")
	}()

	select {
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "Channel remained empty.")
	case <-comp.SubscribeIntegration():
	}

	select {
	case log := <-comp.Subscribe():
		assert.Equal(t, "test log", log.Log)
		assert.Equal(t, "integration1", log.IntegrationID)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "Expected channel to receive logs, but got nothing")
	}

	assert.Equal(t, 2, callbackCount, "Callback not called for both register and send.")
}

func TestSendWithoutRegister(t *testing.T) {
	comp := NewLogsIntegration(logmock.New(t), configmock.New(t))
	callbackCalled := false
	comp.SetActionCallback(func() error {
		callbackCalled = true
		return nil
	})

	// SendLog should not block when the integration is not registered
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 10*time.Millisecond, func() bool {
		comp.SendLog("test log", "integration1")
		return true
	})

	assert.False(t, callbackCalled, "Callback called without registering.")
}

// TestReceiveEmptyConfig ensures that ReceiveIntegration doesn't send an empty
// configuration to subscribers
func TestReceiveEmptyConfig(t *testing.T) {
	logsIntegration := NewLogsIntegration(logmock.New(t), configmock.New(t))
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
