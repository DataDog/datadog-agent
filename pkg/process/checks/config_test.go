// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestEnsureValidMaxBatchSize(t *testing.T) {
	tests := []struct {
		name                 string
		override             bool
		maxPerMessage        int
		expectedMaxBatchSize int
	}{
		{
			name:                 "valid batch count",
			maxPerMessage:        50,
			expectedMaxBatchSize: 50,
		},
		{
			name:                 "negative batch count",
			maxPerMessage:        -1,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "0 max batch size",
			maxPerMessage:        0,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "big max batch size",
			maxPerMessage:        2000,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
	}

	assert := assert.New(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(tc.expectedMaxBatchSize, ensureValidMaxBatchSize(tc.maxPerMessage))
		})
	}
}

func TestEnsureValidMaxBatchBytes(t *testing.T) {
	tests := []struct {
		name                  string
		maxMessageBytes       int
		expectedMaxBatchBytes int
	}{
		{
			name:                  "valid batch size",
			maxMessageBytes:       100000,
			expectedMaxBatchBytes: 100000,
		},
		{
			name:                  "negative batch size",
			maxMessageBytes:       -1,
			expectedMaxBatchBytes: ddconfig.DefaultProcessMaxMessageBytes,
		},
		{
			name:                  "0 max batch size",
			maxMessageBytes:       0,
			expectedMaxBatchBytes: ddconfig.DefaultProcessMaxMessageBytes,
		},
		{
			name:                  "big max batch size",
			maxMessageBytes:       2000000,
			expectedMaxBatchBytes: ddconfig.DefaultProcessMaxMessageBytes,
		},
	}

	assert := assert.New(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(tc.expectedMaxBatchBytes, ensureValidMaxBatchBytes(tc.maxMessageBytes))
		})
	}
}
