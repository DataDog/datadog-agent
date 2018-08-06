// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

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

func TestFindSourceWithInvalidContainerLabelShouldReturnNil(t *testing.T) {
	var source *config.LogSource
	var container *Container

	container = NewContainer(types.Container{Image: "myapp", Labels: map[string]string{"com.datadoghq.ad.logs": "{\"source\":\"any_source\",\"service\":\"any_service\"}"}})
	source = container.findSource(nil)
	assert.Nil(t, source)
}

func TestIsImageMatch(t *testing.T) {
	var container *Container

	container = NewContainer(types.Container{Image: "myapp"})
	assert.True(t, container.isImageMatch("myapp"))

	container = NewContainer(types.Container{Image: "repository/myapp"})
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("repository/myapp"))

	container = NewContainer(types.Container{Image: "myapp@sha256:1234567890"})
	assert.True(t, container.isImageMatch("myapp"))

	container = NewContainer(types.Container{Image: "repository/myapp@sha256:1234567890"})
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("repository/myapp"))

	container = NewContainer(types.Container{Image: "repository/myapp:latest"})
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("myapp:latest"))
	assert.True(t, container.isImageMatch("repository/myapp"))
	assert.True(t, container.isImageMatch("repository/myapp:latest"))

	container = NewContainer(types.Container{Image: "myapp:latest"})
	assert.True(t, container.isImageMatch("myapp"))
	assert.True(t, container.isImageMatch("myapp:latest"))

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

	container = NewContainer(types.Container{Image: "repository/myapp2:latest"})
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(types.Container{Image: "myapp2:latest"})
	assert.False(t, container.isImageMatch("myapp"))

	container = NewContainer(types.Container{Image: "myapp"})
	assert.False(t, container.isImageMatch("dd/myapp"))
	assert.False(t, container.isImageMatch("myapp:latest"))
	assert.False(t, container.isImageMatch("dd/myapp:latest"))

	container = NewContainer(types.Container{Image: "repository/myapp"})
	assert.False(t, container.isImageMatch("dd/myapp"))
	assert.False(t, container.isImageMatch("myapp:latest"))
	assert.False(t, container.isImageMatch("dd/myapp:latest"))

	container = NewContainer(types.Container{Image: "repository/myapp:latest"})
	assert.False(t, container.isImageMatch("dd/myapp"))
	assert.False(t, container.isImageMatch("myapp:foo"))
	assert.False(t, container.isImageMatch("repository/myapp:foo"))

	container = NewContainer(types.Container{Image: "myapp:latest"})
	assert.False(t, container.isImageMatch("myapp:foo"))
	assert.False(t, container.isImageMatch("repository/myapp"))
	assert.False(t, container.isImageMatch("repository/myapp:foo"))
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

func TestIsNameMatch(t *testing.T) {
	var container *Container

	container = NewContainer(types.Container{Names: []string{"foo", "bar"}})
	assert.True(t, container.isNameMatch("foo"))
	assert.True(t, container.isNameMatch("bar"))
	assert.True(t, container.isNameMatch(""))
	assert.False(t, container.isNameMatch("boo"))

	container = NewContainer(types.Container{Names: []string{"/api/v1/pods/foo", "/bar"}})
	assert.True(t, container.isNameMatch("foo"))
	assert.True(t, container.isNameMatch("bar"))
	assert.True(t, container.isNameMatch(""))
	assert.False(t, container.isNameMatch("boo"))
}

func TestFindSourceFromLabelWithWrongLabelNameShouldFail(t *testing.T) {
	var labels map[string]string
	var source *config.LogSource
	var container *Container

	container = NewContainer(types.Container{Labels: labels})
	source = container.findSource(nil)
	assert.Nil(t, source)

	labels = map[string]string{"com.datadoghq.ad.name": "any_name"}
	container = NewContainer(types.Container{Labels: labels})
	source = container.findSource(nil)
	assert.Nil(t, source)
}

func TestFindSourceFromLabelWithWrongFormatShouldFail(t *testing.T) {
	var labels map[string]string
	var source *config.LogSource
	var container *Container

	labels = map[string]string{"com.datadoghq.ad.logs": "{}"}
	container = NewContainer(types.Container{Labels: labels})
	source = container.findSource(nil)
	assert.Nil(t, source)

	labels = map[string]string{"com.datadoghq.ad.logs": "{\"source\":\"any_source\",\"service\":\"any_service\"}"}
	container = NewContainer(types.Container{Labels: labels})
	source = container.findSource(nil)
	assert.Nil(t, source)
}

func TestFindSourceFromLabelWithInvalidProcessingRuleShouldFail(t *testing.T) {
	var labels map[string]string
	var source *config.LogSource
	var container *Container

	labels = map[string]string{"com.datadoghq.ad.logs": `[{"source":"any_source","service":"any_service","log_processing_rules":[{"type":"multi_line"}]}]`}
	container = NewContainer(types.Container{Labels: labels})
	source = container.findSource(nil)
	assert.Nil(t, source)
}

func TestFindSourceFromLabelWithValidFormatShouldSucceed(t *testing.T) {
	var labels map[string]string
	var source *config.LogSource
	var container *Container
	var rule config.ProcessingRule

	labels = map[string]string{"com.datadoghq.ad.logs": `[{}]`}
	container = NewContainer(types.Container{Labels: labels, Image: "any_image"})
	source = container.findSource(nil)
	assert.NotNil(t, source)
	assert.Equal(t, "any_image", source.Name)

	labels = map[string]string{"com.datadoghq.ad.logs": `[{"source":"any_source","service":"any_service"}]`}
	container = NewContainer(types.Container{Labels: labels, Image: "any_image"})
	source = container.findSource(nil)
	assert.NotNil(t, source)
	assert.Equal(t, "any_image", source.Name)
	assert.Equal(t, "any_source", source.Config.Source)
	assert.Equal(t, "any_service", source.Config.Service)

	labels = map[string]string{"com.datadoghq.ad.logs": `[{"source":"any_source","service":"any_service","log_processing_rules":[{"type":"multi_line","name":"numbers","pattern":"[0-9]"}]}]`}
	container = NewContainer(types.Container{Labels: labels})
	source = container.findSource(nil)
	assert.NotNil(t, source)
	assert.Equal(t, "", source.Name)
	assert.Equal(t, "any_source", source.Config.Source)
	assert.Equal(t, "any_service", source.Config.Service)
	assert.Equal(t, 1, len(source.Config.ProcessingRules))
	rule = source.Config.ProcessingRules[0]
	assert.Equal(t, "multi_line", rule.Type)
	assert.Equal(t, "numbers", rule.Name)
	assert.True(t, rule.Reg.MatchString("123"))
	assert.False(t, rule.Reg.MatchString("a123"))
}
