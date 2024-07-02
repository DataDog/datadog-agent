// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package impl defines the OpenTelemetry Extension implementation.
package impl

import (
	"context"
	"testing"

	converter "github.com/DataDog/datadog-agent/comp/otelcol/converter/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/extension/impl/internal/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/extension"
)

func getTestFactory(t *testing.T) extension.Factory {
	conv, err := converter.NewConverter()
	require.NoError(t, err)

	return NewFactory(conv)

}

func TestNewFactory(t *testing.T) {
	factory := getTestFactory(t)
	assert.NotNil(t, factory)

	cfg := factory.CreateDefaultConfig()
	assert.NotNil(t, cfg)

	ext, err := factory.CreateExtension(context.Background(), extension.Settings{}, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, ext)

	_, ok := ext.(*ddExtension)
	assert.True(t, ok)
}

func TestTypeStability(t *testing.T) {
	factory := getTestFactory(t)
	assert.NotNil(t, factory)

	typ := factory.Type()
	assert.Equalf(t, typ, metadata.Type,
		"Factory type is %v expected it to be %x", typ, metadata.Type)

	stability := factory.ExtensionStability()
	assert.Equalf(t, stability, metadata.ExtensionStability,
		"Factory stability is %v expected it to be %x", stability, metadata.ExtensionStability)

}
