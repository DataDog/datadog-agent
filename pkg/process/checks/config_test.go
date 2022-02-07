// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestGetMaxBatchSize(t *testing.T) {
	tests := []struct {
		name                 string
		override             bool
		maxPerMessage        int
		expectedMaxBatchSize int
	}{
		{
			name:                 "default batch size",
			override:             false,
			maxPerMessage:        50,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "valid batch size override",
			override:             true,
			maxPerMessage:        50,
			expectedMaxBatchSize: 50,
		},
		{
			name:                 "negative max batch size",
			override:             true,
			maxPerMessage:        -1,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "0 max batch size",
			override:             true,
			maxPerMessage:        0,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
		{
			name:                 "big max batch size",
			override:             true,
			maxPerMessage:        2000,
			expectedMaxBatchSize: ddconfig.DefaultProcessMaxPerMessage,
		},
	}

	assert := assert.New(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock()
			if tc.override {
				mockConfig.Set("process_config.max_per_message", tc.maxPerMessage)
			}

			// override maxBatchSizeOnce so maxBatchSize can be set to the new value
			maxBatchSizeOnce = sync.Once{}
			assert.Equal(tc.expectedMaxBatchSize, getMaxBatchSize())
		})
	}
}

func TestGetMaxCtrProcsBatchSize(t *testing.T) {
	tests := []struct {
		name                         string
		override                     bool
		maxCtrProcsPerMessage        int
		expectedMaxCtrProcsBatchSize int
	}{
		{
			name:                         "default batch size",
			override:                     false,
			maxCtrProcsPerMessage:        50,
			expectedMaxCtrProcsBatchSize: ddconfig.DefaultProcessMaxCtrProcsPerMessage,
		},
		{
			name:                         "valid batch size override",
			override:                     true,
			maxCtrProcsPerMessage:        50,
			expectedMaxCtrProcsBatchSize: 50,
		},
		{
			name:                         "negative max batch size",
			override:                     true,
			maxCtrProcsPerMessage:        -1,
			expectedMaxCtrProcsBatchSize: ddconfig.DefaultProcessMaxCtrProcsPerMessage,
		},
		{
			name:                         "0 max batch size",
			override:                     true,
			maxCtrProcsPerMessage:        0,
			expectedMaxCtrProcsBatchSize: ddconfig.DefaultProcessMaxCtrProcsPerMessage,
		},
		{
			name:                         "big max batch size",
			override:                     true,
			maxCtrProcsPerMessage:        50000,
			expectedMaxCtrProcsBatchSize: ddconfig.DefaultProcessMaxCtrProcsPerMessage,
		},
	}

	assert := assert.New(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock()
			if tc.override {
				mockConfig.Set("process_config.max_ctr_procs_per_message", tc.maxCtrProcsPerMessage)
			}

			// override maxCtrProcsBatchSizeOnce so maxCtrProcsBatchSize can be set to the new value
			maxCtrProcsBatchSizeOnce = sync.Once{}
			assert.Equal(tc.expectedMaxCtrProcsBatchSize, getMaxCtrProcsBatchSize())
		})
	}
}
