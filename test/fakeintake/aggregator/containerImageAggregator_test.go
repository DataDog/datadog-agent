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
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/contimage_bytes
var ContainerImageData []byte

func TestNewContainerImagePayloads(t *testing.T) {
	t.Run("parseContainerImagePayload empty message should be ignored", func(t *testing.T) {
		payloads, err := ParseContainerImagePayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		require.NoError(t, err)
		require.Empty(t, payloads)
	})

	t.Run("parseContainerImagePayload valid body should parse payloads", func(t *testing.T) {
		payloads, err := ParseContainerImagePayload(api.Payload{Data: ContainerImageData, Encoding: encodingGzip})
		require.NoError(t, err)
		require.Equal(t, 10, len(payloads))

		assert.Equal(t, "gcr.io/datadoghq/agent", payloads[0].name())
		expectedTags := []string{"git.repository_url:https://github.com/DataDog/datadog-agent", "os_name:linux", "architecture:amd64", "image_id:gcr.io/datadoghq/agent@sha256:c08324052945874a0a5fb1ba5d4d5d1d3b8ff1a87b7b3e46810c8cf476ea4c3d", "image_name:gcr.io/datadoghq/agent", "short_image:agent", "image_tag:7.50.1"}
		assert.Equal(t, expectedTags, payloads[0].GetTags())

		assert.Equal(t, "gcr.io/datadoghq/dogstatsd", payloads[2].name())
		expectedTags = []string{"os_name:linux", "architecture:amd64", "git.repository_url:https://github.com/DataDog/datadog-agent", "image_id:gcr.io/datadoghq/dogstatsd@sha256:1d85318569be685c6aeb98df85da995050feda8e55e8f5e122ad96689e6205cd", "image_name:gcr.io/datadoghq/dogstatsd", "short_image:dogstatsd", "image_tag:7.50.1"}
		assert.Equal(t, expectedTags, payloads[2].GetTags())
	})
}
