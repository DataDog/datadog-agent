// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"testing"

	"github.com/docker/docker/api/types"
	types_container "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestFindSourceWithSourceFiltersShouldSucceed(t *testing.T) {
	var source *sources.LogSource
	var container *Container

	sources := []*sources.LogSource{
		sources.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Image: "myapp"}),
		sources.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Label: "mylabel"}),
		sources.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Image: "myapp", Label: "mylabel"}),
		sources.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Identifier: "1234567890"}),
	}

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "myapp"}},
		&service.Service{})
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[0])

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config: &types_container.Config{
			Image:  "wrongapp",
			Labels: map[string]string{"mylabel": "anything"}}},
		&service.Service{})
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[1])

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config: &types_container.Config{
			Image:  "myapp",
			Labels: map[string]string{"mylabel": "anything"}}},
		&service.Service{})
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[2])

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{ID: "1234567890"},
		Config:            &types_container.Config{Labels: map[string]string{"com.datadoghq.ad.logs": "[{}]"}}},
		&service.Service{})
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[3])

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{ID: "0987654321"},
		Config:            &types_container.Config{Labels: map[string]string{"com.datadoghq.ad.logs": "[{}]"}}},
		&service.Service{})
	source = container.FindSource(sources)
	assert.Nil(t, source)

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "wrongapp"}},
		&service.Service{})
	source = container.FindSource(sources)
	assert.Nil(t, source)
}

func TestFindSourceWithNoSourceFilterShouldSucceed(t *testing.T) {
	var source *sources.LogSource
	var container *Container

	sources := []*sources.LogSource{
		sources.NewLogSource("", &config.LogsConfig{Type: config.DockerType}),
		sources.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Label: "mylabel"}),
	}

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config: &types_container.Config{
			Image:  "myapp",
			Labels: map[string]string{"mylabel": "anything"}}},
		&service.Service{})
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[1])

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "wrongapp"}},
		&service.Service{})
	source = container.FindSource(sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[0])
}

func TestIsImageMatch(t *testing.T) {
	var container *Container

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "myapp"}},
		nil)
	assert.True(t, container.isImageMatch("myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "repository/myapp"}},
		nil)
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("repository/myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "myapp@sha256:1234567890"}},
		nil)
	assert.True(t, container.isImageMatch("myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "repository/myapp@sha256:1234567890"}},
		nil)
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("repository/myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "repository/myapp:latest"}},
		nil)
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("myapp:latest"))
	assert.True(t, container.isImageMatch("repository/myapp"))
	assert.True(t, container.isImageMatch("repository/myapp:latest"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "myapp:latest"}},
		nil)
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("myapp:latest"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "repositorymyapp"}},
		nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "myapp2"}},
		nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "myapp2@sha256:1234567890"}},
		nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "repository/myapp2"}},
		nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "repository/myapp2@sha256:1234567890"}},
		nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "repository/myapp2:latest"}},
		nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "myapp2:latest"}},
		nil)
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "myapp"}},
		nil)
	assert.False(t, container.isImageMatch("dd/myapp"))
	assert.False(t, container.isImageMatch("myapp:latest"))
	assert.False(t, container.isImageMatch("dd/myapp:latest"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "repository/myapp"}},
		nil)
	assert.False(t, container.isImageMatch("dd/myapp"))
	assert.False(t, container.isImageMatch("myapp:latest"))
	assert.False(t, container.isImageMatch("dd/myapp:latest"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "repository/myapp:latest"}},
		nil)
	assert.False(t, container.isImageMatch("dd/myapp"))
	assert.False(t, container.isImageMatch("myapp:foo"))
	assert.False(t, container.isImageMatch("repository/myapp:foo"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Image: "myapp:latest"}},
		nil)
	assert.False(t, container.isImageMatch("myapp:foo"))
	assert.False(t, container.isImageMatch("repository/myapp"))
	assert.False(t, container.isImageMatch("repository/myapp:foo"))
}

func TestIsLabelMatch(t *testing.T) {
	var container *Container

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"bar": ""}}},
		nil)
	assert.False(t, container.isLabelMatch("foo"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": ""}}},
		nil)
	assert.True(t, container.isLabelMatch("foo"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": "bar"}}},
		nil)
	assert.True(t, container.isLabelMatch("foo"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"bar": ""}}},
		nil)
	assert.False(t, container.isLabelMatch("foo:bar"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": ""}}},
		nil)
	assert.False(t, container.isLabelMatch("foo:bar"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": "bar"}}},
		nil)
	assert.True(t, container.isLabelMatch("foo:bar"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo:bar": ""}}},
		nil)
	assert.True(t, container.isLabelMatch("foo:bar"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": ""}}},
		nil)
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": "bar"}}},
		nil)
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": "bar:baz"}}},
		nil)
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo:bar": "baz"}}},
		nil)
	assert.False(t, container.isLabelMatch("foo:bar:baz"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo:bar:baz": ""}}},
		nil)
	assert.True(t, container.isLabelMatch("foo:bar:baz"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"bar": ""}}},
		nil)
	assert.False(t, container.isLabelMatch("foo=bar"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": ""}}},
		nil)
	assert.False(t, container.isLabelMatch("foo=bar"))
	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": "bar"}}},
		nil)
	assert.True(t, container.isLabelMatch("foo=bar"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{},
		Config:            &types_container.Config{Labels: map[string]string{"foo": "bar"}}},
		nil)
	assert.True(t, container.isLabelMatch(" a , b:c , foo:bar , d=e "))
}

func TestIsNameMatch(t *testing.T) {
	var container *Container

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{Name: "foo"}},
		nil)
	assert.True(t, container.isNameMatch("foo"))
	assert.True(t, container.isNameMatch(""))
	assert.False(t, container.isNameMatch("boo"))

	container = NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{Name: "/api/v1/pods/foo"}},
		nil)
	assert.True(t, container.isNameMatch("foo"))
	assert.True(t, container.isNameMatch(""))
	assert.False(t, container.isNameMatch("boo"))
}

func TestIsIdentifierMatch(t *testing.T) {
	container := NewContainer(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{ID: "1234567890"}},
		nil)
	assert.True(t, container.isIdentifierMatch("1234567890"))
	assert.False(t, container.isNameMatch(""))
	assert.False(t, container.isNameMatch("docker://1234567890"))
	assert.False(t, container.isNameMatch("0987654321"))
}
