// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build test

package corechecks

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockLongRunningCheck helps to mock the LongRunningCheck interface for testing purposes.
type mockLongRunningCheck struct {
	mock.Mock

	stopCh    chan struct{}
	runningCh chan struct{}
}

func (m *mockLongRunningCheck) Stop() {
	m.Called()
}

func (m *mockLongRunningCheck) Configure(senderManger sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	args := m.Called(senderManger, integrationConfigDigest, config, initConfig, source)
	return args.Error(0)
}

func (m *mockLongRunningCheck) Interval() time.Duration {
	args := m.Called()
	return args.Get(0).(time.Duration)
}

func (m *mockLongRunningCheck) ID() checkid.ID {
	args := m.Called()
	return args.Get(0).(checkid.ID)
}

func (m *mockLongRunningCheck) GetWarnings() []error {
	args := m.Called()
	return args.Get(0).([]error)
}

func (m *mockLongRunningCheck) GetSenderStats() (stats.SenderStats, error) {
	args := m.Called()
	return args.Get(0).(stats.SenderStats), args.Error(1)
}

func (m *mockLongRunningCheck) Version() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockLongRunningCheck) ConfigSource() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockLongRunningCheck) IsTelemetryEnabled() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *mockLongRunningCheck) InitConfig() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockLongRunningCheck) InstanceConfig() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockLongRunningCheck) GetDiagnoses() ([]diagnosis.Diagnosis, error) {
	m.Called()
	return nil, nil
}

func (m *mockLongRunningCheck) GetSender() (sender.Sender, error) {
	args := m.Called()
	s := args.Get(0)
	if s == nil {
		return nil, args.Error(1)
	}
	return s.(sender.Sender), args.Error(1)
}

func newMockLongRunningCheck() *mockLongRunningCheck {
	return &mockLongRunningCheck{
		stopCh:    make(chan struct{}),
		runningCh: make(chan struct{}, 1),
	}
}

func (m *mockLongRunningCheck) Run() error {
	args := m.Called()
	m.runningCh <- struct{}{}
	<-m.stopCh
	return args.Error(0)
}

func (m *mockLongRunningCheck) Cancel() {
	m.Called()
	select {
	case m.stopCh <- struct{}{}:
	default:
	}
}

func (m *mockLongRunningCheck) waitUntilRun() {
	<-m.runningCh
}

// TestLongRunningCheckWrapperRun tests the Run function for different scenarios.
func TestLongRunningCheckWrapperRun(t *testing.T) {
	t.Run("Running a check that is not already started", func(t *testing.T) {
		mockCheck := newMockLongRunningCheck()
		mockCheck.On("Run").Return(nil)
		mockCheck.On("Cancel").Return()

		wrapper := NewLongRunningCheckWrapper(mockCheck)
		err := wrapper.Run()
		assert.Nil(t, err)
		mockCheck.waitUntilRun()

		mockCheck.Cancel()
		mockCheck.AssertExpectations(t)
	})

	t.Run("Committing the sender if the check is already running", func(t *testing.T) {
		mockSender := mocksender.NewMockSender("ok")
		mockSender.On("Commit").Return()

		mockCheck := newMockLongRunningCheck()
		mockCheck.On("GetSender").Return(mockSender, nil)

		wrapper := NewLongRunningCheckWrapper(mockCheck)
		wrapper.running = true // simulate that the check is already running

		err := wrapper.Run()

		assert.Nil(t, err)
		mockSender.AssertExpectations(t)
		mockCheck.AssertExpectations(t)
	})

	t.Run("Returning an error if GetSender fails while already running", func(t *testing.T) {
		mockCheck := newMockLongRunningCheck()
		expectedErr := fmt.Errorf("failed to get sender")
		mockCheck.On("GetSender").Return(nil, expectedErr)

		wrapper := NewLongRunningCheckWrapper(mockCheck)
		wrapper.running = true // simulate that the check is already running

		err := wrapper.Run()

		assert.EqualError(t, err, "error getting sender: failed to get sender")
		mockCheck.AssertExpectations(t)
	})

	t.Run("Make sure innerCheck.Cancel is called only once", func(t *testing.T) {
		mockCheck := newMockLongRunningCheck()
		mockCheck.On("Run").Return(nil)
		mockCheck.On("Cancel").Return()

		wrapper := NewLongRunningCheckWrapper(mockCheck)
		err := wrapper.Run()
		mockCheck.waitUntilRun()

		assert.Nil(t, err)
		wrapper.Cancel()
		wrapper.Cancel()
		mockCheck.AssertNumberOfCalls(t, "Cancel", 1)
	})

	t.Run("Make sure innerCheck.Run can't be called after a cancel", func(t *testing.T) {
		mockCheck := newMockLongRunningCheck()
		mockCheck.On("Run").Return(nil)
		mockCheck.On("Cancel").Return()

		wrapper := NewLongRunningCheckWrapper(mockCheck)
		err := wrapper.Run()
		assert.Nil(t, err)

		wrapper.Cancel()
		err = wrapper.Run()
		assert.Error(t, err)
		mockCheck.waitUntilRun()

		mockCheck.AssertExpectations(t)
		mockCheck.AssertNumberOfCalls(t, "Run", 1)
	})
}

// TestLongRunningCheckWrapperGetSenderStats tests the GetSenderStats function.
func TestLongRunningCheckWrapperGetSenderStats(t *testing.T) {
	mockCheck := newMockLongRunningCheck()
	expectedStats := stats.SenderStats{MetricSamples: 10, Events: 2}
	mockCheck.On("GetSenderStats").Return(expectedStats, nil)

	wrapper := NewLongRunningCheckWrapper(mockCheck)
	senderStats, err := wrapper.GetSenderStats()

	assert.Nil(t, err)
	assert.True(t, senderStats.LongRunningCheck)
	assert.Equal(t, expectedStats.MetricSamples, senderStats.MetricSamples)
	assert.Equal(t, expectedStats.Events, senderStats.Events)
	mockCheck.AssertExpectations(t)
}
