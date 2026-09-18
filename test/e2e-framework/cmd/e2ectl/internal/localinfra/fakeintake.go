// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localinfra holds the helpers shared by the Pulumi-free local
// drivers: the local docker fakeintake and the routable-IP resolution.
package localinfra

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/fakeintakeconfig"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"net"
	"os"
	"os/exec"
)

// fakeintakeImage shares the profile override and reproducible pin with cloud fixtures.
func fakeintakeImage() string {
	override, _ := runner.GetProfile().ParamStore().GetWithDefault(parameters.FakeintakeImageOverride, "")
	return fakeintakeconfig.ImageURL("public.ecr.aws/datadog/fakeintake", override)
}

// RunFakeintake starts the local fakeintake docker container on a free port
// and returns the port. The container binds every interface: pods must reach
// the fakeintake through the host IP, so binding loopback only would hide it
// from the cluster.
func RunFakeintake(container string) (int, error) {
	port, err := freePort()
	if err != nil {
		return 0, err
	}
	cmd := exec.Command("docker", "run", "-d", "--name", container,
		"-p", fmt.Sprintf("%d:80", port),
		fakeintakeImage(),
		"--remoteconfig=true",
		"--rc-key-data="+outputs.DefaultRCSigningKeySeed,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("starting fakeintake: %w", err)
	}
	return port, nil
}

// StopFakeintake removes the local fakeintake container (no error when absent).
func StopFakeintake(container string) error {
	_ = exec.Command("docker", "rm", "-f", container).Run()
	return nil
}

// OutboundIP returns an IP routable from containers on this host (the
// "UDP dial" trick used by the e2e-framework local fakeintake component).
func OutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
