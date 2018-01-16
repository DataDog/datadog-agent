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

	cfg = &config.IntegrationConfigLogSource{Type: config.DockerType, Image: "myapp", Label: "mylabel"}
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

func (suite *ContainerScannerTestSuite) TestContainerLabelFilter() {

	suite.False(suite.shouldMonitor("foo", map[string]string{"bar": ""}))
	suite.True(suite.shouldMonitor("foo", map[string]string{"foo": ""}))
	suite.True(suite.shouldMonitor("foo", map[string]string{"foo": "bar"}))

	suite.False(suite.shouldMonitor("foo:bar", map[string]string{"bar": ""}))
	suite.False(suite.shouldMonitor("foo:bar", map[string]string{"foo": ""}))
	suite.True(suite.shouldMonitor("foo:bar", map[string]string{"foo": "bar"}))
	suite.True(suite.shouldMonitor("foo:bar", map[string]string{"foo:bar": ""}))

	suite.False(suite.shouldMonitor("foo:bar:baz", map[string]string{"foo": ""}))
	suite.False(suite.shouldMonitor("foo:bar:baz", map[string]string{"foo": "bar"}))
	suite.False(suite.shouldMonitor("foo:bar:baz", map[string]string{"foo": "bar:baz"}))
	suite.False(suite.shouldMonitor("foo:bar:baz", map[string]string{"foo:bar": "baz"}))
	suite.True(suite.shouldMonitor("foo:bar:baz", map[string]string{"foo:bar:baz": ""}))

	suite.False(suite.shouldMonitor("foo=bar", map[string]string{"bar": ""}))
	suite.False(suite.shouldMonitor("foo=bar", map[string]string{"foo": ""}))
	suite.True(suite.shouldMonitor("foo=bar", map[string]string{"foo": "bar"}))

	suite.True(suite.shouldMonitor(" a , b:c , foo:bar , d=e ", map[string]string{"foo": "bar"}))

}

func (suite *ContainerScannerTestSuite) shouldMonitor(configLabel string, containerLabels map[string]string) bool {
	cfg := &config.IntegrationConfigLogSource{Type: config.DockerType, Label: configLabel}
	container := types.Container{Labels: containerLabels}
	return suite.c.sourceShouldMonitorContainer(cfg, container)
}

func TestContainerScannerTestSuite(t *testing.T) {
	suite.Run(t, new(ContainerScannerTestSuite))
}
