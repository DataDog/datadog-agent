// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func getTestFactory(t *testing.T) extension.Factory {
	factories, err := components()
	assert.NoError(t, err)

	return NewFactoryForAgent(&factories, newConfigProviderSettings(uriFromFile("config.yaml"), false), option.New[ipc.Component](ipcmock.New(t)), false)
}

func TestNewFactoryForAgent(t *testing.T) {
	factory := getTestFactory(t)
	require.NotNil(t, factory)

	cfg := factory.CreateDefaultConfig()
	require.NotNil(t, cfg)

	settings := extension.Settings{TelemetrySettings: componenttest.NewNopTelemetrySettings()}
	ext, err := factory.Create(t.Context(), settings, cfg)
	require.NoError(t, err)
	require.NotNil(t, ext)
	t.Cleanup(func() {
		require.NoError(t, ext.Shutdown(t.Context()))
	})

	_, ok := ext.(*ddExtension)
	assert.True(t, ok)
}

func TestTypeStability(t *testing.T) {
	factory := getTestFactory(t)
	require.NotNil(t, factory)

	typ := factory.Type()
	assert.Equalf(t, typ, metadata.Type,
		"Factory type is %v expected it to be %x", typ, metadata.Type)

	stability := factory.Stability()
	assert.Equalf(t, stability, metadata.ExtensionStability,
		"Factory stability is %v expected it to be %x", stability, metadata.ExtensionStability)
}
