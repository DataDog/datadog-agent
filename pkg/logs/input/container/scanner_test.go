// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package container

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/suite"
)

type ContainerScannerTestSuite struct {
	suite.Suite
	c *Scanner
}

func (suite *ContainerScannerTestSuite) SetupTest() {
	suite.c = &Scanner{}
}

func (suite *ContainerScannerTestSuite) TestContainerScannerFilter() {
	cfg := &config.IntegrationConfigLogSource{Type: config.DockerType, Image: "myapp"}
	container := types.Container{Image: "myapp"}
	suite.True(suite.c.sourceShouldMonitorContainer(cfg, container))
	container = types.Container{Image: "myapp2"}
	suite.False(suite.c.sourceShouldMonitorContainer(cfg, container))

	cfg = &config.IntegrationConfigLogSource{Type: config.DockerType, Label: "mylabel"}
	l1 := make(map[string]string)
	l2 := make(map[string]string)
	l2["mylabel"] = "anything"
	container = types.Container{Image: "myapp", Labels: l1}
	suite.False(suite.c.sourceShouldMonitorContainer(cfg, container))
	container = types.Container{Image: "myapp", Labels: l2}
	suite.True(suite.c.sourceShouldMonitorContainer(cfg, container))

	cfg = &config.IntegrationConfigLogSource{Type: config.DockerType}
	suite.True(suite.c.sourceShouldMonitorContainer(cfg, container))
}

func TestContainerScannerTestSuite(t *testing.T) {
	suite.Run(t, new(ContainerScannerTestSuite))
}
