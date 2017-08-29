// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build linux

package dogstatsd

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

const (
	socatImg  string = "datadog/socat-proxy:master"
	socatName string = "dd-test-socat-proxy"
)

// FIXME: move as a system test once the runner is able to run them

// testUDSOriginDetection ensures UDS origin detection works, by submitting
// a metric from a `socat` container. As we need the origin PID to stay running,
// we can't just `netcat` to the socket, that's why we keep socat running as
// UDP->UDS proxy and submit the metric through it.
//
// FIXME: this test should be ported to the go docker client
func testUDSOriginDetection(t *testing.T) {
	dir, err := ioutil.TempDir("", "dd-test-")
	assert.Nil(t, err)
	defer os.RemoveAll(dir) // clean up
	socketPath := filepath.Join(dir, "dsd.socket")
	config.Datadog.Set("dogstatsd_socket", socketPath)
	config.Datadog.Set("dogstatsd_origin_detection", true)
	var contents = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

	// Build proxy docker image
	buildCmd := exec.Command("docker", "build",
		"-t", socatImg,
		"../../../Dockerfiles/dogstatsd/socat-proxy/")
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Logf("Error building docker image: %s", string(output))
		panic(err)
	}
	rmImageCmd := exec.Command("docker", "image", "rm", socatImg)
	defer rmImageCmd.Run()

	// Start DSD
	packetChannel := make(chan *listeners.Packet)
	s, err := listeners.NewUDSListener(packetChannel)
	if err != nil {
		panic(err)
	}
	go s.Listen()
	defer s.Stop()

	// Run proxy docker image
	runCmd := exec.Command("docker", "run", "-d", "--rm",
		"-v", fmt.Sprintf("%s:/socket/statsd.socket", socketPath),
		socatImg)
	output, err = runCmd.CombinedOutput()
	if err != nil {
		t.Logf("Error running docker image: %s", string(output))
		panic(err)
	}
	containerId := strings.Trim(string(output), "\n")
	assert.Equal(t, 64, len(containerId))

	// Get socat's IP
	socatIp, err := utils.GetContainerIP(containerId)
	if err != nil {
		t.Logf("Error getting socat's IP: %s", string(output))
		panic(err)
	}

	t.Logf("Running socat container: %s on IP %s", containerId, socatIp)
	stopCmd := exec.Command("docker", "stop", containerId)
	defer stopCmd.Run()

	// Send test data through proxy via UDP
	conn, err := net.Dial("udp", fmt.Sprintf("%s:8125", socatIp))
	assert.Nil(t, err)
	defer conn.Close()
	conn.Write(contents)

	select {
	case packet := <-packetChannel:
		assert.NotNil(t, packet)
		assert.Equal(t, contents, packet.Contents)
		assert.Equal(t, fmt.Sprintf("docker://%s", containerId), packet.Origin)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}
