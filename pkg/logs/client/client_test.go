// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDestinationMetadata(t *testing.T) {
	meta := NewDestinationMetadata("component", "instance", "http", "endpoint1", "category1")
	require.NotNil(t, meta)
	assert.True(t, meta.ReportingEnabled)
}

func TestNewNoopDestinationMetadata(t *testing.T) {
	meta := NewNoopDestinationMetadata()
	require.NotNil(t, meta)
	assert.False(t, meta.ReportingEnabled)
}

func TestDestinationMetadataTelemetryName(t *testing.T) {
	t.Run("reporting enabled", func(t *testing.T) {
		meta := NewDestinationMetadata("comp", "inst", "http", "ep1", "cat")
		name := meta.TelemetryName()
		assert.Equal(t, "comp_inst_http_ep1", name)
	})

	t.Run("reporting disabled", func(t *testing.T) {
		meta := NewNoopDestinationMetadata()
		name := meta.TelemetryName()
		assert.Equal(t, "", name)
	})
}

func TestDestinationMetadataMonitorTag(t *testing.T) {
	t.Run("reporting enabled", func(t *testing.T) {
		meta := NewDestinationMetadata("comp", "inst", "tcp", "ep2", "cat")
		tag := meta.MonitorTag()
		assert.Equal(t, "destination_tcp_ep2", tag)
	})

	t.Run("reporting disabled", func(t *testing.T) {
		meta := NewNoopDestinationMetadata()
		tag := meta.MonitorTag()
		assert.Equal(t, "", tag)
	})
}

func TestDestinationMetadataEvpCategory(t *testing.T) {
	meta := NewDestinationMetadata("comp", "inst", "http", "ep", "my_category")
	assert.Equal(t, "my_category", meta.EvpCategory())
}

func TestNewDestinations(t *testing.T) {
	reliable := []Destination{}
	unreliable := []Destination{}
	dests := NewDestinations(reliable, unreliable)
	require.NotNil(t, dests)
	assert.NotNil(t, dests.Reliable)
	assert.NotNil(t, dests.Unreliable)
}

func TestRetryableError(t *testing.T) {
	t.Run("create and get error message", func(t *testing.T) {
		originalErr := errors.New("connection timeout")
		retryErr := NewRetryableError(originalErr)
		require.NotNil(t, retryErr)
		assert.Equal(t, "connection timeout", retryErr.Error())
	})

	t.Run("implements error interface", func(t *testing.T) {
		originalErr := errors.New("test error")
		retryErr := NewRetryableError(originalErr)
		var err error = retryErr
		assert.NotNil(t, err)
		assert.Equal(t, "test error", err.Error())
	})
}
