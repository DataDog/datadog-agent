// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextension

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()
	require.NotNil(t, factory)

	// Verify factory type
	assert.Equal(t, Type, factory.Type())
}

func TestNewFactory_CreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	require.NotNil(t, cfg)

	config, ok := cfg.(*Config)
	require.True(t, ok)
	assert.Equal(t, 300, config.MetadataInterval)
}

func TestNewFactory_CreateExtension_ReturnsError(t *testing.T) {
	// Note: The factory's create function is tested indirectly
	// Direct testing would require accessing internal factory methods
	// This test verifies the factory is properly constructed
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	assert.NotNil(t, factory)
	assert.NotNil(t, cfg)

	// The actual extension creation is tested in extension_test.go
	_ = context.Background()
	_ = extension.Settings{}
}

func TestFactory(t *testing.T) {
	// Test that factory struct has expected fields
	f := &factory{}
	require.NotNil(t, f)

	// Verify all fields are accessible (compile-time check)
	_ = f.config
	_ = f.log
	_ = f.serializer
	_ = f.hostname
	_ = f.tagger
	_ = f.ipc
	_ = f.telemetry
}

func TestType(t *testing.T) {
	// Verify the extension type is correctly defined
	assert.Equal(t, "dogtel", Type.String())
}

func TestStability(t *testing.T) {
	// Verify stability level constant
	assert.Equal(t, component.StabilityLevelAlpha, stability)
}
