// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gnmi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestSubscribeCommandOneShot(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"gnmi", "subscribe", "--address", "router-1", "--interval", "3s"},
		runSubscribe,
		func(_ core.BundleParams, params *cliParams) {
			assert.Equal(t, "router-1", params.address)
			assert.Equal(t, 3*time.Second, params.interval)
			assert.True(t, params.useTLS)
		})
}

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
	subscribe := commands[0].Commands()[0]
	assert.Equal(t, "subscribe", subscribe.Name())
	assert.Equal(t, "true", subscribe.Flags().Lookup("use-tls").DefValue)
}

func TestValidateSubscribeParams(t *testing.T) {
	require.Error(t, validateSubscribeParams(nil))
	require.Error(t, validateSubscribeParams(&cliParams{}))
	require.Error(t, validateSubscribeParams(&cliParams{interval: -time.Second}))
	require.NoError(t, validateSubscribeParams(&cliParams{interval: time.Second}))
}
