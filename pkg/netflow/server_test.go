// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package netflow

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/netflow/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartServerAndStopServer(t *testing.T) {
	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(log, 10*time.Millisecond)
	defer demux.Stop(false)

	port := testutil.GetFreePort()

	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.MergeConfigOverride(strings.NewReader(fmt.Sprintf(`
network_devices:
  netflow:
    enabled: true
    listeners:
      - flow_type: netflow5
        bind_host: 127.0.0.1
        port: %d
`, port)))
	require.NoError(t, err)
	config.Datadog.Set("hostname", "my-hostname")

	err = StartServer(demux)
	require.NoError(t, err)
	require.NotNil(t, serverInstance)

	replaceWithDummyFlowProcessor(serverInstance, 123)

	StopServer()
	require.Nil(t, serverInstance)
}

func TestIsEnabled(t *testing.T) {
	saved := config.Datadog.Get("network_devices.netflow.enabled")
	defer config.Datadog.Set("network_devices.netflow.enabled", saved)

	config.Datadog.Set("network_devices.netflow.enabled", true)
	assert.Equal(t, true, IsEnabled())

	config.Datadog.Set("network_devices.netflow.enabled", false)
	assert.Equal(t, false, IsEnabled())
}

func TestServer_Stop(t *testing.T) {
	// Setup NetFlow config
	port := testutil.GetFreePort()

	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.MergeConfigOverride(strings.NewReader(fmt.Sprintf(`
network_devices:
  netflow:
    enabled: true
    aggregator_flush_interval: 1
    listeners:
      - flow_type: netflow5 # netflow, sflow, ipfix
        bind_host: 0.0.0.0
        port: %d # default 2055 for netflow
`, port)))
	require.NoError(t, err)

	// Setup Netflow Server
	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(log, 10*time.Millisecond)
	defer demux.Stop(false)
	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	server, err := NewNetflowServer(sender, nil)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	flowProcessor := replaceWithDummyFlowProcessor(server, port)

	// Stops server
	server.stop()

	// Assert logs present
	assert.Equal(t, flowProcessor.stopped, true)
}
