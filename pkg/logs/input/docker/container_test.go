// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"testing"

	"github.com/docker/docker/api/types"
	dockerConfig "github.com/docker/docker/api/types/container"
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
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Identifier: "1234567890"}),
	}

	container = NewContainer(newContainerJSON("myapp", "", "", nil), nil)
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[0])

	container = NewContainer(newContainerJSON("wrongapp", "", "", map[string]string{"mylabel": "anything"}), nil)
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[1])

	container = NewContainer(newContainerJSON("myapp", "", "", map[string]string{"mylabel": "anything"}), nil)
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[2])

	container = NewContainer(newContainerJSON("", "1234567890", "", map[string]string{"com.datadoghq.ad.logs": "[{}]"}), nil)
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[3])

	container = NewContainer(newContainerJSON("", "0987654321", "", map[string]string{"com.datadoghq.ad.logs": "[{}]"}), nil)
	source = container.FindSource(sources)
	assert.Nil(t, source)

	container = NewContainer(types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{Image: "wrongapp"}}, nil)
	source = container.FindSource(sources)
	assert.Nil(t, source)
}

func TestFindSourceWithNoSourceFilterShouldSucceed(t *testing.T) {
	var source *config.LogSource
	var container *Container

	sources := []*config.LogSource{
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType}),
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Label: "mylabel"}),
	}

	container = NewContainer(newContainerJSON("myapp", "", "", map[string]string{"mylabel": "anything"}), nil)
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[1])

	container = NewContainer(newContainerJSON("wrongapp", "", "", nil), nil)
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[0])
}

func TestIsImageMatch(t *testing.T) {
	var container *Container

	container = NewContainer(newContainerJSON("myapp", "", "", nil), nil)
	assert.True(t, container.isImageMatch("myapp"))

	container = NewContainer(newContainerJSON("repository/myapp", "", "", nil), nil)
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("repository/myapp"))

	container = NewContainer(newContainerJSON("myapp@sha256:1234567890", "", "", nil), nil)
	assert.True(t, container.isImageMatch("myapp"))

	container = NewContainer(newContainerJSON("repository/myapp@sha256:1234567890", "", "", nil), nil)
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("repository/myapp"))

	container = NewContainer(newContainerJSON("repository/myapp:latest", "", "", nil), nil)
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("myapp:latest"))
	assert.True(t, container.isImageMatch("repository/myapp"))
	assert.True(t, container.isImageMatch("repository/myapp:latest"))

	container = NewContainer(newContainerJSON("myapp:latest", "", "", nil), nil)
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("myapp:latest"))

	container = NewContainer(newContainerJSON("repositorymyapp", "", "", nil), nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(newContainerJSON("myapp2", "", "", nil), nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(newContainerJSON("myapp2@sha256:1234567890", "", "", nil), nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(newContainerJSON("repository/myapp2", "", "", nil), nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(newContainerJSON("repository/myapp2@sha256:1234567890", "", "", nil), nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(newContainerJSON("repository/myapp2:latest", "", "", nil), nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(newContainerJSON("myapp2:latest", "", "", nil), nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(newContainerJSON("myapp", "", "", nil), nil)
	assert.False(t, container.isImageMatch("dd/myapp"))
	assert.False(t, container.isImageMatch("myapp:latest"))
	assert.False(t, container.isImageMatch("dd/myapp:latest"))

	container = NewContainer(newContainerJSON("repository/myapp", "", "", nil), nil)
	assert.False(t, container.isImageMatch("dd/myapp"))
	assert.False(t, container.isImageMatch("myapp:latest"))
	assert.False(t, container.isImageMatch("dd/myapp:latest"))

	container = NewContainer(newContainerJSON("repository/myapp:latest", "", "", nil), nil)
	assert.False(t, container.isImageMatch("dd/myapp"))
	assert.False(t, container.isImageMatch("myapp:foo"))
	assert.False(t, container.isImageMatch("repository/myapp:foo"))

	container = NewContainer(newContainerJSON("myapp:latest", "", "", nil), nil)
	assert.False(t, container.isImageMatch("myapp:foo"))
	assert.False(t, container.isImageMatch("repository/myapp"))
	assert.False(t, container.isImageMatch("repository/myapp:foo"))
}

func TestIsLabelMatch(t *testing.T) {
	var container *Container

	container = NewContainer(newContainerJSON("", "", "", map[string]string{"bar": ""}), nil)
	assert.False(t, container.isLabelMatch("foo"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": ""}), nil)
	assert.True(t, container.isLabelMatch("foo"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": "bar"}), nil)
	assert.True(t, container.isLabelMatch("foo"))

	container = NewContainer(newContainerJSON("", "", "", map[string]string{"bar": ""}), nil)
	assert.False(t, container.isLabelMatch("foo:bar"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": ""}), nil)
	assert.False(t, container.isLabelMatch("foo:bar"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": "bar"}), nil)
	assert.True(t, container.isLabelMatch("foo:bar"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo:bar": ""}), nil)
	assert.True(t, container.isLabelMatch("foo:bar"))

	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": ""}), nil)
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": "bar"}), nil)
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": "bar:baz"}), nil)
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo:bar": "baz"}), nil)
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo:bar:baz": ""}), nil)
	assert.True(t, container.isLabelMatch("foo:bar:baz"))

	container = NewContainer(newContainerJSON("", "", "", map[string]string{"bar": ""}), nil)
	assert.False(t, container.isLabelMatch("foo=bar"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": ""}), nil)
	assert.False(t, container.isLabelMatch("foo=bar"))
	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": "bar"}), nil)
	assert.True(t, container.isLabelMatch("foo=bar"))

	container = NewContainer(newContainerJSON("", "", "", map[string]string{"foo": "bar"}), nil)
	assert.True(t, container.isLabelMatch(" a , b:c , foo:bar , d=e "))
}

func TestIsNameMatch(t *testing.T) {
	var container *Container

	container = NewContainer(newContainerJSON("", "", "foo", nil), nil)
	assert.True(t, container.isNameMatch("foo"))
	assert.True(t, container.isNameMatch(""))
	assert.False(t, container.isNameMatch("boo"))

	container = NewContainer(newContainerJSON("", "", "/api/v1/pods/foo", nil), nil)
	assert.True(t, container.isNameMatch("foo"))
	assert.True(t, container.isNameMatch(""))
	assert.False(t, container.isNameMatch("boo"))
}

func TestIsIdentifierMatch(t *testing.T) {
	container := NewContainer(newContainerJSON("", "1234567890", "", nil), nil)
	assert.True(t, container.isIdentifierMatch("1234567890"))
	assert.False(t, container.isNameMatch(""))
	assert.False(t, container.isNameMatch("docker://1234567890"))
	assert.False(t, container.isNameMatch("0987654321"))
}

func newContainerJSON(image string, id string, name string, labels map[string]string) types.ContainerJSON {
	return types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{Image: image, ID: id, Name: name},
		Config:            &dockerConfig.Config{Labels: labels},
	}
}
