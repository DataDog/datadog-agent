// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package container

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestIsImageMatch(t *testing.T) {
	assert.True(t, isImageMatch("myapp", "myapp"))
	assert.True(t, isImageMatch("myapp", "repository/myapp"))
	assert.True(t, isImageMatch("myapp", "myapp@sha256:1234567890"))
	assert.True(t, isImageMatch("myapp", "repository/myapp@sha256:1234567890"))

	assert.False(t, isImageMatch("myapp", "repositorymyapp"))
	assert.False(t, isImageMatch("myapp", "myapp2"))
	assert.False(t, isImageMatch("myapp", "myapp2@sha256:1234567890"))
	assert.False(t, isImageMatch("myapp", "repository/myapp2"))
	assert.False(t, isImageMatch("myapp", "repository/myapp2@sha256:1234567890"))

	// cfg = config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Image: "myapp", Label: "mylabel"})
	// l1 := make(map[string]string)
	// l2 := make(map[string]string)
	// l2["mylabel"] = "anything"
	// container = types.Container{Image: "myapp", Labels: l1}
	// assert.False(assert.c.sourceShouldMonitorContainer(cfg, container))
	// container = types.Container{Image: "myapp", Labels: l2}
	// assert.True(assert.c.sourceShouldMonitorContainer(cfg, container))

	// cfg = config.NewLogSource("", &config.LogsConfig{Type: config.DockerType})
	// assert.True(assert.c.sourceShouldMonitorContainer(cfg, container))
}

func TestIsLabelMatch(t *testing.T) {
	assert.False(t, isLabelMatch("foo", map[string]string{"bar": ""}))
	assert.True(t, isLabelMatch("foo", map[string]string{"foo": ""}))
	assert.True(t, isLabelMatch("foo", map[string]string{"foo": "bar"}))

	assert.False(t, isLabelMatch("foo:bar", map[string]string{"bar": ""}))
	assert.False(t, isLabelMatch("foo:bar", map[string]string{"foo": ""}))
	assert.True(t, isLabelMatch("foo:bar", map[string]string{"foo": "bar"}))
	assert.True(t, isLabelMatch("foo:bar", map[string]string{"foo:bar": ""}))

	assert.False(t, isLabelMatch("foo:bar:baz", map[string]string{"foo": ""}))
	assert.False(t, isLabelMatch("foo:bar:baz", map[string]string{"foo": "bar"}))
	assert.False(t, isLabelMatch("foo:bar:baz", map[string]string{"foo": "bar:baz"}))
	assert.False(t, isLabelMatch("foo:bar:baz", map[string]string{"foo:bar": "baz"}))
	assert.True(t, isLabelMatch("foo:bar:baz", map[string]string{"foo:bar:baz": ""}))

	assert.False(t, isLabelMatch("foo=bar", map[string]string{"bar": ""}))
	assert.False(t, isLabelMatch("foo=bar", map[string]string{"foo": ""}))
	assert.True(t, isLabelMatch("foo=bar", map[string]string{"foo": "bar"}))

	assert.True(t, isLabelMatch(" a , b:c , foo:bar , d=e ", map[string]string{"foo": "bar"}))
}

func TestExtractLogsConfigWithNoValidKeyShouldFail(t *testing.T) {
	var labels map[string]string
	var config *config.LogsConfig

	config = extractLogsConfig(labels)
	assert.Nil(t, config)

	labels = map[string]string{"com.datadoghq.ad.name": "any_name"}
	config = extractLogsConfig(labels)
	assert.Nil(t, config)
}

func TestExtractLogsConfigWithWrongFormatShouldFail(t *testing.T) {
	var labels map[string]string
	var config *config.LogsConfig

	labels = map[string]string{"com.datadoghq.ad.logs": "{}"}
	config = extractLogsConfig(labels)
	assert.Nil(t, config)

	labels = map[string]string{"com.datadoghq.ad.logs": "{\"source\":\"any_source\",\"service\":\"any_service\"}"}
	config = extractLogsConfig(labels)
	assert.Nil(t, config)
}

func TestExtractLogsConfigWithValidFormatShouldSucceed(t *testing.T) {
	var labels map[string]string
	var config *config.LogsConfig

	labels = map[string]string{"com.datadoghq.ad.logs": "[{}]"}
	config = extractLogsConfig(labels)
	assert.NotNil(t, config)

	labels = map[string]string{"com.datadoghq.ad.logs": "[{\"source\":\"any_source\",\"service\":\"any_service\"}]"}
	config = extractLogsConfig(labels)
	assert.NotNil(t, config)
	assert.Equal(t, "any_source", config.Source)
	assert.Equal(t, "any_service", config.Service)
}
