// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	WebsiteStack = "WEBSITE_STACK"
	AppLogsTrace = "WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED"
)

func TestInAzureAppServices(t *testing.T) {
	os.Setenv(WebsiteStack, " ")
	isLinuxAzure := inAzureAppServices()
	os.Unsetenv(WebsiteStack)

	os.Setenv(AppLogsTrace, " ")
	isWindowsAzure := inAzureAppServices()
	os.Unsetenv(AppLogsTrace)

	isNotAzure := inAzureAppServices()

	assert.True(t, isLinuxAzure)
	assert.True(t, isWindowsAzure)
	assert.False(t, isNotAzure)
}

func TestPeerTagsAggregation(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg := New()
		cfg.PeerTagsAggregation = false
		assert.False(t, cfg.PeerTagsAggregation)
		assert.Empty(t, cfg.PeerTags)
		assert.Empty(t, cfg.ConfiguredPeerTags())
	})

	t.Run("default-enabled", func(t *testing.T) {
		cfg := New()
		assert.Empty(t, cfg.PeerTags)
		assert.Equal(t, basePeerTags, cfg.ConfiguredPeerTags())
	})
	t.Run("disabled-user-tags", func(t *testing.T) {
		cfg := New()
		cfg.PeerTagsAggregation = false
		cfg.PeerTags = []string{"user_peer_tag"}
		assert.False(t, cfg.PeerTagsAggregation)
		assert.Empty(t, cfg.ConfiguredPeerTags())
	})
	t.Run("enabled-user-tags", func(t *testing.T) {
		cfg := New()
		cfg.PeerTags = []string{"user_peer_tag"}
		assert.Equal(t, append(basePeerTags, "user_peer_tag"), cfg.ConfiguredPeerTags())
	})
	t.Run("dedup", func(t *testing.T) {
		cfg := New()
		cfg.PeerTags = basePeerTags[:2]
		assert.Equal(t, basePeerTags, cfg.ConfiguredPeerTags())
	})
}
