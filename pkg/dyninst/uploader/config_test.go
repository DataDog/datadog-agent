// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		c := defaultConfig()
		assert.Equal(t, http.DefaultClient, c.client)
		assert.Equal(t, defaultMaxBatchItems, c.batcherConfig.maxBatchItems)
		assert.Equal(t, defaultMaxBatchSizeBytes, c.batcherConfig.maxBatchSizeBytes)
		assert.Equal(t, defaultMaxBufferDuration, c.batcherConfig.maxBufferDuration)
	})

	t.Run("with client", func(t *testing.T) {
		c := defaultConfig()
		client := &http.Client{}
		WithClient(client)(&c)
		assert.Equal(t, client, c.client)
	})

	t.Run("with max batch items", func(t *testing.T) {
		c := defaultConfig()
		WithMaxBatchItems(10)(&c)
		assert.Equal(t, 10, c.batcherConfig.maxBatchItems)
	})

	t.Run("with max batch size bytes", func(t *testing.T) {
		c := defaultConfig()
		WithMaxBatchSizeBytes(1000)(&c)
		assert.Equal(t, 1000, c.batcherConfig.maxBatchSizeBytes)
	})

	t.Run("with max buffer duration", func(t *testing.T) {
		c := defaultConfig()
		WithMaxBufferDuration(10 * time.Millisecond)(&c)
		assert.Equal(t, 10*time.Millisecond, c.batcherConfig.maxBufferDuration)
	})
}
