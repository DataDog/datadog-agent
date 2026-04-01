// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package dogtelextensionimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

// TestNewFactory_ReturnsErrorOnCreate verifies NewFactory produces a factory
// that refuses to create an extension (agent components are required).
func TestNewFactory_ReturnsErrorOnCreate(t *testing.T) {
	factory := NewFactory()
	assert.Equal(t, Type, factory.Type())

	cfg := factory.CreateDefaultConfig()
	require.NotNil(t, cfg)

	settings := extension.Settings{ID: component.NewID(Type)}
	_, err := factory.Create(context.Background(), settings, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dogtelextension requires agent components")
}

// TestNewFactoryForAgent_CreatesExtension verifies NewFactoryForAgent produces
// a working factory that creates an extension successfully.
func TestNewFactoryForAgent_CreatesExtension(t *testing.T) {
	hostname, _ := hostnameinterface.NewMock("test-host")
	factory := NewFactoryForAgent(
		configmock.NewMockWithOverrides(t, nil),
		logmock.New(t),
		serializermock.NewMetricSerializer(t),
		hostname,
		nil, // workloadmeta
		nil, // tagger
		ipcmock.New(t),
		noopsimpl.GetCompatComponent(),
		nil, // secrets
	)

	assert.Equal(t, Type, factory.Type())

	cfg := factory.CreateDefaultConfig()
	settings := extension.Settings{
		ID:        component.NewID(Type),
		BuildInfo: component.BuildInfo{Version: "1.0.0", Command: "otel-agent"},
	}
	ext, err := factory.Create(context.Background(), settings, cfg)
	require.NoError(t, err)
	require.NotNil(t, ext)
}

// TestNewFactoryForAgent_InvalidConfigErrors verifies the factory rejects
// an invalid configuration at create time.
func TestNewFactoryForAgent_InvalidConfigErrors(t *testing.T) {
	hostname, _ := hostnameinterface.NewMock("test-host")
	factory := NewFactoryForAgent(
		configmock.NewMockWithOverrides(t, nil),
		logmock.New(t),
		serializermock.NewMetricSerializer(t),
		hostname,
		nil,
		nil,
		ipcmock.New(t),
		noopsimpl.GetCompatComponent(),
		nil,
	)

	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.TaggerServerPort = -5

	settings := extension.Settings{ID: component.NewID(Type)}
	_, err := factory.Create(context.Background(), settings, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid configuration")
}

// TestNewExtension_FX_InvalidConfig verifies the FX constructor (NewExtension)
// rejects an invalid default config.
func TestNewExtension_FX_DefaultSucceeds(t *testing.T) {
	hostname, _ := hostnameinterface.NewMock("test-host")
	ext, err := NewExtension(Requires{
		Config:     configmock.NewMockWithOverrides(t, nil),
		Log:        logmock.New(t),
		Serializer: serializermock.NewMetricSerializer(t),
		Hostname:   hostname,
		IPC:        ipcmock.New(t),
		Telemetry:  noopsimpl.GetCompatComponent(),
	})
	require.NoError(t, err)
	require.NotNil(t, ext)
}
