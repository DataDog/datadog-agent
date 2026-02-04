// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package dogtelextension

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/extension"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestTaggerServerWrapper_Interface(t *testing.T) {
	// Test that taggerServerWrapper implements the required interface
	wrapper := &taggerServerWrapper{
		ext: nil,
	}

	// Compile-time check that wrapper implements pb.AgentSecureServer
	var _ pb.AgentSecureServer = wrapper
	assert.NotNil(t, wrapper)
}

func TestExtension_StopTaggerServer_NilServer(t *testing.T) {
	ctx := context.Background()
	components := createMockComponents(t, true)

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, extension.Settings{}, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Stop without starting should not panic
	assert.NotPanics(t, func() {
		dogtelExt.stopTaggerServer()
	})
}

func TestExtension_StopTaggerServer_MultipleCalls(t *testing.T) {
	ctx := context.Background()
	components := createMockComponents(t, true)

	cfg := &Config{
		MetadataInterval:        300,
		EnableTaggerServer:      false,
		TaggerServerPort:        0,
		TaggerServerAddr:        "localhost",
		TaggerMaxMessageSize:    4 * 1024 * 1024,
		TaggerMaxConcurrentSync: 5,
	}

	f := &factory{
		config:     components.config,
		log:        components.log,
		serializer: components.serializer,
		hostname:   components.hostname,
		tagger:     components.tagger,
		ipc:        components.ipc,
		telemetry:  components.telemetry,
	}
	ext, err := newExtension(ctx, extension.Settings{}, cfg, f)
	require.NoError(t, err)

	dogtelExt := ext.(*dogtelExtension)

	// Multiple stops should not panic even if never started
	assert.NotPanics(t, func() {
		dogtelExt.stopTaggerServer()
		dogtelExt.stopTaggerServer()
	})
}
