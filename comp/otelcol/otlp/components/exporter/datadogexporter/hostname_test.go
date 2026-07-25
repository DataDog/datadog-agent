// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package datadogexporter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
)

// TestHostnameService_Get verifies the adapter surfaces the hostname resolved by
// the underlying source.Provider through all three interface methods.
func TestHostnameService_Get(t *testing.T) {
	hs := newHostnameService(serializerexporter.SourceProviderFunc(func(context.Context) (string, error) {
		return "my-host", nil
	}))

	got, err := hs.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "my-host", got)
	assert.Equal(t, "my-host", hs.GetSafe(context.Background()))

	data, err := hs.GetWithProvider(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "my-host", data.Hostname)
}

// TestHostnameService_Error verifies error propagation and the GetSafe fallback.
func TestHostnameService_Error(t *testing.T) {
	hs := newHostnameService(serializerexporter.SourceProviderFunc(func(context.Context) (string, error) {
		return "", errors.New("boom")
	}))

	_, err := hs.Get(context.Background())
	require.Error(t, err)
	assert.Equal(t, "unknown host", hs.GetSafe(context.Background()))
}
