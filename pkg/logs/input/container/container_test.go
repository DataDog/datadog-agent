// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package container

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestFindSourceWithSourceFiltersShouldSucceed(t *testing.T) {
	var source *config.LogSource
	var container *Container

	sources := []*config.LogSource{
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Image: "myapp"}),
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Label: "mylabel"}),
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Image: "myapp", Label: "mylabel"}),
	}

	container = NewContainer(types.Container{Image: "myapp"})
	source = container.findSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[0])

	container = NewContainer(types.Container{Image: "wrongapp", Labels: map[string]string{"mylabel": "anything"}})
	source = container.findSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[1])

	container = NewContainer(types.Container{Image: "myapp", Labels: map[string]string{"mylabel": "anything"}})
	source = container.findSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[2])

	container = NewContainer(types.Container{Image: "wrongapp"})
	source = container.findSource(sources)
	assert.Nil(t, source)
}

func TestFindSourceWithNoSourceFilterShouldSucceed(t *testing.T) {
	var source *config.LogSource
	var container *Container

	sources := []*config.LogSource{
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType}),
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Label: "mylabel"}),
	}

	container = NewContainer(types.Container{Image: "myapp", Labels: map[string]string{"mylabel": "anything"}})
	source = container.findSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[1])

	container = NewContainer(types.Container{Image: "wrongapp"})
	source = container.findSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[0])

	container = NewContainer(types.Container{Image: "wrongapp", Labels: map[string]string{"wronglabel": "anything", "com.datadoghq.ad.logs": "[{\"source\":\"any_source\",\"service\":\"any_service\"}]"}})
	source = container.findSource(sources)
	assert.NotNil(t, source)
	for _, s := range sources {
		assert.NotEqual(t, source, s)
	}
}

func TestIsImageMatch(t *testing.T) {
	var container *Container

	container = NewContainer(types.Container{Image: "myapp"})
	assert.True(t, container.isImageMatch("myapp"))
	container = NewContainer(types.Container{Image: "repository/myapp"})
	assert.True(t, container.isImageMatch("myapp"))
	container = NewContainer(types.Container{Image: "myapp@sha256:1234567890"})
	assert.True(t, container.isImageMatch("myapp"))
	container = NewContainer(types.Container{Image: "repository/myapp@sha256:1234567890"})
	assert.True(t, container.isImageMatch("myapp"))

	container = NewContainer(types.Container{Image: "repositorymyapp"})
	assert.False(t, container.isImageMatch("myapp"))
	container = NewContainer(types.Container{Image: "myapp2"})
	assert.False(t, container.isImageMatch("myapp"))
	container = NewContainer(types.Container{Image: "myapp2@sha256:1234567890"})
	assert.False(t, container.isImageMatch("myapp"))
	container = NewContainer(types.Container{Image: "repository/myapp2"})
	assert.False(t, container.isImageMatch("myapp"))
	container = NewContainer(types.Container{Image: "repository/myapp2@sha256:1234567890"})
	assert.False(t, container.isImageMatch("myapp"))
}

func TestIsLabelMatch(t *testing.T) {
	var container *Container

	container = NewContainer(types.Container{Labels: map[string]string{"bar": ""}})
	assert.False(t, container.isLabelMatch("foo"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo": ""}})
	assert.True(t, container.isLabelMatch("foo"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo": "bar"}})
	assert.True(t, container.isLabelMatch("foo"))

	container = NewContainer(types.Container{Labels: map[string]string{"bar": ""}})
	assert.False(t, container.isLabelMatch("foo:bar"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo": ""}})
	assert.False(t, container.isLabelMatch("foo:bar"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo": "bar"}})
	assert.True(t, container.isLabelMatch("foo:bar"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo:bar": ""}})
	assert.True(t, container.isLabelMatch("foo:bar"))

	container = NewContainer(types.Container{Labels: map[string]string{"foo": ""}})
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo": "bar"}})
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo": "bar:baz"}})
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo:bar": "baz"}})
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo:bar:baz": ""}})
	assert.True(t, container.isLabelMatch("foo:bar:baz"))

	container = NewContainer(types.Container{Labels: map[string]string{"bar": ""}})
	assert.False(t, container.isLabelMatch("foo=bar"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo": ""}})
	assert.False(t, container.isLabelMatch("foo=bar"))
	container = NewContainer(types.Container{Labels: map[string]string{"foo": "bar"}})
	assert.True(t, container.isLabelMatch("foo=bar"))

	container = NewContainer(types.Container{Labels: map[string]string{"foo": "bar"}})
	assert.True(t, container.isLabelMatch(" a , b:c , foo:bar , d=e "))
}

func TestParseConfigWithNoValidKeyShouldFail(t *testing.T) {
	var labels map[string]string
	var config *config.LogsConfig
	var container *Container

	container = NewContainer(types.Container{Labels: labels})
	config = container.parseConfig()
	assert.Nil(t, config)

	labels = map[string]string{"com.datadoghq.ad.name": "any_name"}
	container = NewContainer(types.Container{Labels: labels})
	config = container.parseConfig()
	assert.Nil(t, config)
}

func TestParseConfigWithWrongFormatShouldFail(t *testing.T) {
	var labels map[string]string
	var config *config.LogsConfig
	var container *Container

	labels = map[string]string{"com.datadoghq.ad.logs": "{}"}
	container = NewContainer(types.Container{Labels: labels})
	config = container.parseConfig()
	assert.Nil(t, config)

	labels = map[string]string{"com.datadoghq.ad.logs": "{\"source\":\"any_source\",\"service\":\"any_service\"}"}
	container = NewContainer(types.Container{Labels: labels})
	config = container.parseConfig()
	assert.Nil(t, config)
}

func TestParseConfigWithValidFormatShouldSucceed(t *testing.T) {
	var labels map[string]string
	var config *config.LogsConfig
	var container *Container

	labels = map[string]string{"com.datadoghq.ad.logs": "[{}]"}
	container = NewContainer(types.Container{Labels: labels})
	config = container.parseConfig()
	assert.NotNil(t, config)

	labels = map[string]string{"com.datadoghq.ad.logs": "[{\"source\":\"any_source\",\"service\":\"any_service\"}]"}
	container = NewContainer(types.Container{Labels: labels})
	config = container.parseConfig()
	assert.NotNil(t, config)
	assert.Equal(t, "any_source", config.Source)
	assert.Equal(t, "any_service", config.Service)
}
