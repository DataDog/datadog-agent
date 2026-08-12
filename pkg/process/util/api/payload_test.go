// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
)

func TestEncodePayloadUsesSupportedZstdEncoding(t *testing.T) {
	payload := &model.CollectorProc{HostName: "test-host"}

	encoded, err := EncodePayload(payload)
	if !assert.NoError(t, err) {
		return
	}

	decoded, err := model.DecodeMessage(encoded)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, model.MessageEncodingZstd1xPB, decoded.Header.Encoding)
	assert.Equal(t, payload, decoded.Body)
}
