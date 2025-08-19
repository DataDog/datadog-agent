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

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

//go:embed fixtures/orch_bytes
var orchData []byte

func TestOrchEmptyPayload(t *testing.T) {
	resources, err := ParseOrchestratorPayload(api.Payload{
		Data:     []byte(""),
		Encoding: encodingDeflate,
	})
	assert.Error(t, err)
	assert.Empty(t, resources)
}

func TestOrchInvalidPayload(t *testing.T) {
	resources, err := ParseOrchestratorPayload(api.Payload{
		Data:     []byte("totally a legit payload"),
		Encoding: encodingDeflate,
	})
	assert.Error(t, err)
	assert.Empty(t, resources)
}

func TestOrchNodePayload(t *testing.T) {
	resources, err := ParseOrchestratorPayload(api.Payload{Data: orchData, Encoding: encodingDeflate})
	require.NoError(t, err)
	assert.Equal(t, len(resources), 1)
	assert.Equal(t, resources[0].Name, "kind-control-plane")
	assert.Equal(t, resources[0].Node.PodCIDR, "10.244.0.0/24")
}
