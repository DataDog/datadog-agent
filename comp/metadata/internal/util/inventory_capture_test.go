// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type scheduledInventory struct {
	InventoryCaptureTiming
	testPayload
}

func captureInventoryProvider(t *testing.T) (*InventoryPayload, *telemetrycapture.Manager, *scheduledInventory) {
	t.Helper()
	i := getTestInventoryPayload(t, nil)
	i.createdAt = time.Now().Add(-time.Hour)
	i.MinInterval, i.MaxInterval = 7*time.Second, 20*time.Second
	p := &scheduledInventory{}
	i.getPayload = func() marshaler.JSONMarshaler { return p }
	m := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	t.Cleanup(m.Close)
	i.ConfigureCapture(m, telemetrycapture.AgentInventory)
	return i, m, p
}

func TestInventoryReadinessFollowsActualSubmissionAndCadence(t *testing.T) {
	i, m, p := captureInventoryProvider(t)
	require.Empty(t, m.Status().Capabilities)
	s := i.serializer.(*serializermock.MetricSerializer)
	s.On("SendMetadata", p).Run(func(mock.Arguments) {
		require.Empty(t, m.Status().Capabilities, "readiness must wait for submission")
		at, cadence := p.CaptureInventorySchedule()
		require.Equal(t, i.LastCollect, at)
		require.Equal(t, 21*time.Second, cadence)
	}).Return(nil).Once()
	require.Equal(t, 7*time.Second, i.collect(context.Background()))
	require.Equal(t, []telemetrycapture.Capability{{Stream: telemetrycapture.AgentInventory, Cadence: 21 * time.Second}}, m.Status().Capabilities)
	// An ordinary poll neither collects nor submits again before MaxInterval.
	last := i.LastCollect
	require.Equal(t, i.MinInterval, i.collect(context.Background()))
	require.Equal(t, last, i.LastCollect)
	i.LastCollect = time.Now().Add(-time.Minute)
	s.On("SendMetadata", p).Return(errors.New("submission failed")).Once()
	i.collect(context.Background())
	require.Empty(t, m.Status().Capabilities)
}

func TestInventoryUnavailableAndDisabledProvidersStayUnready(t *testing.T) {
	for _, mode := range []string{"first-delay", "serializer-unavailable", "nil-payload", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			i, m, _ := captureInventoryProvider(t)
			switch mode {
			case "first-delay":
				i.createdAt, i.firstRunDelay = time.Now(), time.Hour
			case "serializer-unavailable":
				i.serializer = nil
			case "nil-payload":
				i.getPayload = func() marshaler.JSONMarshaler { return nil }
			case "disabled":
				i.Enabled = false
			}
			provider := i.MetadataProvider()
			if mode == "disabled" {
				require.Nil(t, provider.Callback)
			} else {
				provider.Callback(context.Background())
			}
			require.Empty(t, m.Status().Capabilities)
		})
	}
}

func TestInventoryShutdownCannotReadvertiseLateSubmission(t *testing.T) {
	for _, closeManager := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider-stop", true: "manager-close"}[closeManager], func(t *testing.T) {
			i, m, p := captureInventoryProvider(t)
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			i.serializer.(*serializermock.MetricSerializer).On("SendMetadata", p).Run(func(mock.Arguments) { close(entered); <-release }).Return(nil).Once()
			go func() { defer close(done); i.collect(context.Background()) }()
			<-entered
			if closeManager {
				m.Close()
			} else {
				i.StopCapture()
			}
			close(release)
			<-done
			require.Empty(t, m.Status().Capabilities)
		})
	}
}
