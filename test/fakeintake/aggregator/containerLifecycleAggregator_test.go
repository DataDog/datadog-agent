// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/contlcycle_bytes
var ContainerLifecycleData []byte

func TestNewContainerLifecyclePayloads(t *testing.T) {
	t.Run("parseContainerLifecyclePayload empty message should be ignored", func(t *testing.T) {
		payloads, err := ParseContainerLifecyclePayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, payloads)
	})

	t.Run("parseContainerLifecyclePayload valid body should parse payloads", func(t *testing.T) {
		payloads, err := ParseContainerLifecyclePayload(api.Payload{Data: ContainerLifecycleData, Encoding: encodingGzip})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(payloads))

		assert.Equal(t, "container_id://14b734660ee253a641373da2d5ece5a092b4f4649986ce9793e922b02936221a", payloads[0].name())
		assert.Equal(t, "container_id://2d00b4758773ff3c97696279214655c715251060db251acc08f53cfd61da998e", payloads[1].name())
	})
}
