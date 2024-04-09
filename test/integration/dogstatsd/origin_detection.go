// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package dogstatsd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

const (
	senderImg string = "datadog/test-origin-detection-sender:master"
)

// testUDSOriginDetection ensures UDS origin detection works, by submitting
// a metric from a container. As we need the origin PID to stay running,
// we can't just `netcat` to the socket, that's why we run a custom python
// script that will stay up after sending packets.
func testUDSOriginDetection(t *testing.T, network string) {
	coreConfig.SetFeatures(t, coreConfig.Docker)

	cfg := map[string]any{}

	t.Logf("Running testUDSOriginDetection with network %s", network)
	// Detect whether we are containerised and set the socket path accordingly
	var socketVolume string
	var composeFile string
	dir := os.ExpandEnv("$SCRATCH_VOLUME_PATH")
	if dir == "" { // Running on the host
		dir = t.TempDir()
		socketVolume = dir
		composeFile = "mount_path.compose"

	} else { // Running in container
		socketVolume = os.ExpandEnv("$SCRATCH_VOLUME_NAME")
		composeFile = "mount_volume.compose"
	}
	socketPath := filepath.Join(dir, "dsd.socket")
	t.Logf("Using socket %s", socketPath)
	if network == "unixgram" {
		cfg["dogstatsd_socket"] = socketPath
	} else if network == "unix" {
		cfg["dogstatsd_stream_socket"] = socketPath
	}
	cfg["dogstatsd_origin_detection"] = true

	confComponent := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: cfg}),
	))

	pidMap := fxutil.Test[pidmap.Component](t, fx.Options(
		pidmapimpl.Module(),
	))

	// Start DSD
	// The packet channel needs to be buffered, otherwise the sender will block and this
	// will prevent disconnections.
	packetsChannel := make(chan packets.Packets, 1024)
	sharedPacketPool := packets.NewPool(32)
	sharedPacketPoolManager := packets.NewPoolManager(sharedPacketPool)
	var err error
	var s listeners.StatsdListener
	if network == "unixgram" {
		s, err = listeners.NewUDSDatagramListener(packetsChannel, sharedPacketPoolManager, nil, confComponent, nil, optional.NewNoneOption[workloadmeta.Component](), pidMap)
	} else if network == "unix" {
		s, err = listeners.NewUDSStreamListener(packetsChannel, sharedPacketPoolManager, nil, confComponent, nil, optional.NewNoneOption[workloadmeta.Component](), pidMap)
	}
	require.NotNil(t, s)
	require.Nil(t, err)

	// Start sender container
	t.Logf("Starting sender container %s", composeFile)
	s.Listen()
	defer s.Stop()

	compose := &utils.ComposeConf{
		ProjectName: "origin-detection-test",
		FilePath:    fmt.Sprintf("testdata/origin_detection/%s", composeFile),
		Variables: map[string]string{
			"socket_dir_path": socketVolume,
			"socket_type":     network,
		},
	}

	output, err := compose.Start()
	defer compose.Stop()
	require.Nil(t, err, string(output))
	t.Logf("Docker output: %s", output)

	containers, err := compose.ListContainers()
	if e, ok := err.(*exec.ExitError); ok {
		require.NoError(t, e, string(e.Stderr))
	}
	require.NoError(t, err)
	require.True(t, len(containers) > 0)
	containerId := containers[0]
	require.Equal(t, 64, len(containerId))

	t.Logf("Running sender container: %s", containerId)
	stopCmd := exec.Command("docker", "stop", containerId)
	defer stopCmd.Run()

	select {
	case packets := <-packetsChannel:
		packet := packets[0]
		require.NotNil(t, packet)
		// The content could be there multiple times, and there could be a \n suffix.
		require.Contains(t, string(packet.Contents), "custom_counter1:1|c")
		require.Equal(t, fmt.Sprintf("container_id://%s", containerId), packet.Origin)
		sharedPacketPool.Put(packet)
	case <-time.After(2 * time.Second):
		// Get container logs to ease debugging
		logsCmd := exec.Command("docker", "logs", containerId)
		output, err = logsCmd.CombinedOutput()
		if err != nil {
			t.Logf("Error getting logs from container %s: %s", containerId, err)
		}
		err = logsCmd.Run()
		if err != nil {
			t.Logf("Error getting logs from container %s: %s", containerId, err)
		}

		t.Logf("Container logs: %s", output)

		assert.FailNow(t, "Timeout on receive channel")
	}
}
