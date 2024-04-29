// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

//go:embed fixtures/orchmanif_bytes
var orchManifData []byte

func TestOrchManifEmptyPayload(t *testing.T) {
	manifs, err := ParseOrchestratorManifestPayload(api.Payload{
		Data:     []byte(""),
		Encoding: encodingDeflate,
	})
	assert.Error(t, err)
	assert.Empty(t, manifs)
}

func TestOrchManifInvalidPayload(t *testing.T) {
	manifs, err := ParseOrchestratorManifestPayload(api.Payload{
		Data:     []byte("totally a legit payload"),
		Encoding: encodingDeflate,
	})
	assert.Error(t, err)
	assert.Empty(t, manifs)
}

func TestOrchManifNodePayload(t *testing.T) {
	manifs, err := ParseOrchestratorManifestPayload(api.Payload{Data: orchManifData, Encoding: encodingDeflate})
	require.NoError(t, err)
	assert.Equal(t, len(manifs), 3)
	assert.Equal(t, manifs[0].Type, process.MessageType(80))
	assert.Equal(t, manifs[0].Manifest.Uid, "fc55f4fc-a5ef-4bda-adc7-c159ea71c17e")
}
