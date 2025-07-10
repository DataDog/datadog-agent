// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/extension"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func getTestFactory(t *testing.T) extension.Factory {
	factories, err := components()
	assert.NoError(t, err)

	return NewFactoryForAgent(&factories, newConfigProviderSettings(uriFromFile("config.yaml"), false), option.None[ipc.Component](), false)
}

func TestNewFactoryForAgent(t *testing.T) {
	factory := getTestFactory(t)
	assert.NotNil(t, factory)

	cfg := factory.CreateDefaultConfig()
	assert.NotNil(t, cfg)

	ext, err := factory.Create(context.Background(), extension.Settings{}, cfg)
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

	stability := factory.Stability()
	assert.Equalf(t, stability, metadata.ExtensionStability,
		"Factory stability is %v expected it to be %x", stability, metadata.ExtensionStability)
}
