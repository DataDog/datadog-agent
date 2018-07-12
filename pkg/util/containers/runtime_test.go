// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build linux

package containers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RuntimeDetectionTestSuite struct {
	suite.Suite
	proc *tempProc
}

func (s *RuntimeDetectionTestSuite) SetupTest() {
	var err error
	s.proc, err = newTempProc("runtime-detection")
	assert.NoError(s.T(), err)
}

func (s *RuntimeDetectionTestSuite) TearDownTest() {
	s.proc.removeAll()
	s.proc = nil
}

func (s *RuntimeDetectionTestSuite) TestContainerd() {
	s.proc.addDummyProcess("1", "0", "/sbin/init")
	s.proc.addDummyProcess("25", "1", "/usr/local/bin/containerd --log-level debug")
	s.proc.addDummyProcess("28", "25", "containerd-shim -namespace k8s.io ...")
	s.proc.addDummyProcess("444", "28", "/opt/datadog-agent/bin/agent/agent start")

	runtime, err := GetRuntimeForPID(444)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), RuntimeNameContainerd, runtime)
}

func (s *RuntimeDetectionTestSuite) TestDockerContainerdK8s() {
	s.proc.addDummyProcess("1", "0", "/sbin/init")
	s.proc.addDummyProcess("25", "1", "/usr/local/bin/docker-containerd --log-level debug")
	s.proc.addDummyProcess("28", "25", "docker-containerd-shim -namespace k8s.io ...")
	s.proc.addDummyProcess("444", "28", "/opt/datadog-agent/bin/agent/agent start")

	runtime, err := GetRuntimeForPID(444)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), RuntimeNameContainerd, runtime)
}

func (s *RuntimeDetectionTestSuite) TestDockerContainerdMoby() {
	s.proc.addDummyProcess("1", "0", "/sbin/init")
	s.proc.addDummyProcess("25", "1", "/usr/local/bin/docker-containerd --log-level debug")
	s.proc.addDummyProcess("28", "25", "docker-containerd-shim -namespace moby ...")
	s.proc.addDummyProcess("444", "28", "/opt/datadog-agent/bin/agent/agent start")

	runtime, err := GetRuntimeForPID(444)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), RuntimeNameDocker, runtime)
}

func (s *RuntimeDetectionTestSuite) TestDockerLegacyCentOS7() {
	s.proc.addDummyProcess("1", "0", "/usr/lib/systemd/systemd")
	s.proc.addDummyProcess("10", "1", "/usr/bin/dockerd-current ...")
	s.proc.addDummyProcess("25", "10", "/usr/bin/docker-containerd-current ...")
	s.proc.addDummyProcess("28", "25", "/usr/bin/docker-containerd-shim-current 6f82f4e18c89fb10d533303220ce192e3a1b4cb6e0b79b01145ab3c5bfeec804 ...")
	s.proc.addDummyProcess("444", "28", "/opt/datadog-agent/bin/agent/agent start")

	runtime, err := GetRuntimeForPID(444)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), RuntimeNameDocker, runtime)
}

func (s *RuntimeDetectionTestSuite) TestDockerLegacyNominal() {
	s.proc.addDummyProcess("1", "0", "/usr/lib/systemd/systemd")
	s.proc.addDummyProcess("10", "1", "/usr/bin/dockerd ...")
	s.proc.addDummyProcess("25", "10", "docker-containerd ...")
	s.proc.addDummyProcess("28", "25", "docker-containerd-shim-current 6f82f4e18c89fb10d533303220ce192e3a1b4cb6e0b79b01145ab3c5bfeec804 ...")
	s.proc.addDummyProcess("444", "28", "/opt/datadog-agent/bin/agent/agent start")

	runtime, err := GetRuntimeForPID(444)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), RuntimeNameDocker, runtime)
}

func (s *RuntimeDetectionTestSuite) TestCRIO() {
	s.proc.addDummyProcess("1", "0", "/usr/lib/systemd/systemd")
	s.proc.addDummyProcess("25", "1", "conmon ...")
	s.proc.addDummyProcess("444", "25", "/opt/datadog-agent/bin/agent/agent start")

	runtime, err := GetRuntimeForPID(444)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), RuntimeNameCRIO, runtime)
}

func (s *RuntimeDetectionTestSuite) TestNoMatch() {
	s.proc.addDummyProcess("1", "0", "/usr/lib/systemd/systemd")
	s.proc.addDummyProcess("25", "1", "supervisord ...")
	s.proc.addDummyProcess("444", "25", "/opt/datadog-agent/bin/agent/agent start")

	runtime, err := GetRuntimeForPID(444)
	assert.Equal(s.T(), ErrNoRuntimeMatch, err)
	assert.Equal(s.T(), "", runtime)
}

func TestRuntimeDetectionTestSuite(t *testing.T) {
	suite.Run(t, new(RuntimeDetectionTestSuite))
}
