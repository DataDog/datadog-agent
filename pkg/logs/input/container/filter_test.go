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

func TestFilter(t *testing.T) {
	var source *config.LogSource
	var containersToTail []*Container

	containers := []types.Container{
		{ID: "1", Image: "myapp"},
		{ID: "2", Image: "myapp", Labels: map[string]string{"mylabel": "anything"}},
		{ID: "3", Image: "wrongapp"},
		{ID: "4", Image: "wrongapp", Labels: map[string]string{"mylabel": "anything"}},
		{ID: "5", Image: "myapp", Labels: map[string]string{"wronglabel": "anything"}},
		{ID: "6", Image: "wrongapp", Labels: map[string]string{"wronglabel": "anything"}},
		{ID: "7", Image: "wrongapp", Labels: map[string]string{"wronglabel": "anything", "com.datadoghq.ad.logs": "[{\"source\":\"any_source\",\"service\":\"any_service\"}]"}},
	}

	source = config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Image: "myapp"})
	containersToTail = Filter(containers, []*config.LogSource{source})
	assert.Equal(t, 4, len(containersToTail))
	assert.Equal(t, "1", containersToTail[0].Identifier)
	assert.Equal(t, source, containersToTail[0].Source)
	assert.Equal(t, "2", containersToTail[1].Identifier)
	assert.Equal(t, source, containersToTail[1].Source)
	assert.Equal(t, "5", containersToTail[2].Identifier)
	assert.Equal(t, source, containersToTail[2].Source)
	assert.Equal(t, "7", containersToTail[3].Identifier)
	assert.NotEqual(t, source, containersToTail[3].Source)

	source = config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Label: "mylabel"})
	containersToTail = Filter(containers, []*config.LogSource{source})
	assert.Equal(t, 3, len(containersToTail))
	assert.Equal(t, "2", containersToTail[0].Identifier)
	assert.Equal(t, source, containersToTail[0].Source)
	assert.Equal(t, "4", containersToTail[1].Identifier)
	assert.Equal(t, source, containersToTail[1].Source)
	assert.Equal(t, "7", containersToTail[2].Identifier)
	assert.NotEqual(t, source, containersToTail[2].Source)

	source = config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Image: "myapp", Label: "mylabel"})
	containersToTail = Filter(containers, []*config.LogSource{source})
	assert.Equal(t, 2, len(containersToTail))
	assert.Equal(t, "2", containersToTail[0].Identifier)
	assert.Equal(t, source, containersToTail[0].Source)
	assert.Equal(t, "7", containersToTail[1].Identifier)
	assert.NotEqual(t, source, containersToTail[1].Source)

	source = config.NewLogSource("", &config.LogsConfig{Type: config.DockerType})
	containersToTail = Filter(containers, []*config.LogSource{source})
	assert.Equal(t, 7, len(containersToTail))
}

func TestSearchSourceWithSourceFiltersShouldSucceed(t *testing.T) {
	var source *config.LogSource
	var container types.Container

	sources := []*config.LogSource{
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Image: "myapp"}),
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Label: "mylabel"}),
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Image: "myapp", Label: "mylabel"}),
	}

	container = types.Container{Image: "myapp"}
	source = searchSource(container, sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[0])

	container = types.Container{Image: "wrongapp", Labels: map[string]string{"mylabel": "anything"}}
	source = searchSource(container, sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[1])

	container = types.Container{Image: "myapp", Labels: map[string]string{"mylabel": "anything"}}
	source = searchSource(container, sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[2])

	container = types.Container{Image: "wrongapp"}
	source = searchSource(container, sources)
	assert.Nil(t, source)
}

func TestSearchSourceWithNoSourceFilterShouldSucceed(t *testing.T) {
	var source *config.LogSource
	var container types.Container

	sources := []*config.LogSource{
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType}),
		config.NewLogSource("", &config.LogsConfig{Type: config.DockerType, Label: "mylabel"}),
	}

	container = types.Container{Image: "myapp", Labels: map[string]string{"mylabel": "anything"}}
	source = searchSource(container, sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[1])

	container = types.Container{Image: "wrongapp"}
	source = searchSource(container, sources)
	assert.NotNil(t, source)
	assert.Equal(t, source, sources[0])

	container = types.Container{Image: "wrongapp", Labels: map[string]string{"wronglabel": "anything", "com.datadoghq.ad.logs": "[{\"source\":\"any_source\",\"service\":\"any_service\"}]"}}
	source = searchSource(container, sources)
	assert.NotNil(t, source)
	for _, s := range sources {
		assert.NotEqual(t, source, s)
	}
}

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
