// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gnmi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
)

func TestFormatCacheKey(t *testing.T) {
	key := formatCacheKey(client.CachedValue{
		Key: client.CacheKey{
			Path: "/system/state/hostname",
		},
	})
	assert.Equal(t, "/system/state/hostname", key)

	key = formatCacheKey(client.CachedValue{
		Key: client.CacheKey{
			Path: "/interfaces/interface/state/counters/in-octets",
			Keys: map[string]string{"name": "eth0"},
		},
	})
	assert.Equal(t, "/interfaces/interface/state/counters/in-octets{name=eth0}", key)
}

func TestCommandsRegistersSubscribe(t *testing.T) {
	commands := Commands(&command.GlobalParams{})
	require.Len(t, commands, 1)
	require.Len(t, commands[0].Commands(), 1)
	assert.Equal(t, "subscribe", commands[0].Commands()[0].Name())
}
