// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestEnsureValidMaxBatchSize(t *testing.T) {
	tests := []struct {
		name                 string
		override             bool
		maxPerMessage        int
		expectedMaxBatchSize int
	}{
		{
			name:                 "valid smaller batch count",
			maxPerMessage:        50,
			expectedMaxBatchSize: 50,
		},
		{
			name:                 "valid larger batch count",
			maxPerMessage:        200,
			expectedMaxBatchSize: 200,
		},
		{
			name:                 "invalid negative batch count",
			maxPerMessage:        -1,
			expectedMaxBatchSize: pkgconfigsetup.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "invalid 0 max batch size",
			maxPerMessage:        0,
			expectedMaxBatchSize: pkgconfigsetup.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "invalid big max batch size",
			maxPerMessage:        20000,
			expectedMaxBatchSize: pkgconfigsetup.DefaultProcessMaxPerMessage,
		},
	}

	assert := assert.New(t)
	for _, tc := range tests {
		t.Run(tc.name, func(_ *testing.T) {
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
			name:                  "valid smaller batch size",
			maxMessageBytes:       100000,
			expectedMaxBatchBytes: 100000,
		},
		{
			name:                  "valid larger batch size",
			maxMessageBytes:       2000000,
			expectedMaxBatchBytes: 2000000,
		},
		{
			name:                  "invalid negative batch size",
			maxMessageBytes:       -1,
			expectedMaxBatchBytes: pkgconfigsetup.DefaultProcessMaxMessageBytes,
		},
		{
			name:                  "invalid 0 max batch size",
			maxMessageBytes:       0,
			expectedMaxBatchBytes: pkgconfigsetup.DefaultProcessMaxMessageBytes,
		},
		{
			name:                  "invalid big max batch size",
			maxMessageBytes:       20000000,
			expectedMaxBatchBytes: pkgconfigsetup.DefaultProcessMaxMessageBytes,
		},
	}

	assert := assert.New(t)
	for _, tc := range tests {
		t.Run(tc.name, func(_ *testing.T) {
			assert.Equal(tc.expectedMaxBatchBytes, ensureValidMaxBatchBytes(tc.maxMessageBytes))
		})
	}
}
