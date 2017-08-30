// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build linux

package dogstatsd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
)

const (
	senderImg string = "datadog/test-origin-detection-sender:master"
)

// testUDSOriginDetection ensures UDS origin detection works, by submitting
// a metric from a container. As we need the origin PID to stay running,
// we can't just `netcat` to the socket, that's why we run a custom python
// script that will stay up after sending packets.
//
// FIXME: this test should be ported to the go docker client
func testUDSOriginDetection(t *testing.T) {
	dir, err := ioutil.TempDir("", "dd-test-")
	assert.Nil(t, err)
	defer os.RemoveAll(dir) // clean up
	socketPath := filepath.Join(dir, "dsd.socket")
	config.Datadog.Set("dogstatsd_socket", socketPath)
	config.Datadog.Set("dogstatsd_origin_detection", true)

	// Build sender docker image
	buildCmd := exec.Command("docker", "build",
		"-t", senderImg,
		"fixtures/origin_detection/")
	output, err := buildCmd.CombinedOutput()
	require.Nil(t, err)

	rmImageCmd := exec.Command("docker", "image", "rm", senderImg)
	defer rmImageCmd.Run()

	// Start DSD
	packetChannel := make(chan *listeners.Packet)
	s, err := listeners.NewUDSListener(packetChannel)
	require.Nil(t, err)

	go s.Listen()
	defer s.Stop()

	// Run sender docker image
	runCmd := exec.Command("docker", "run", "-d", "--rm",
		"-v", fmt.Sprintf("%s:/dsd.socket", socketPath),
		senderImg)
	output, err = runCmd.CombinedOutput()
	require.Nil(t, err)

	containerId := strings.Trim(string(output), "\n")
	require.Equal(t, 64, len(containerId))

	t.Logf("Running sender container: %s", containerId)
	stopCmd := exec.Command("docker", "stop", containerId)
	defer stopCmd.Run()

	select {
	case packet := <-packetChannel:
		require.NotNil(t, packet)
		require.Equal(t, string(packet.Contents), "custom_counter1:1|c")
		require.Equal(t, packet.Origin, fmt.Sprintf("docker://%s", containerId))
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}
