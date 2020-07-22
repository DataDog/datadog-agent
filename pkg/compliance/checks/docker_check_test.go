// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/docker/docker/api/types"

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
)

var (
	mockCtx = mock.Anything
)

func loadTestJSON(path string, obj interface{}) error {
	jsonFile, err := os.Open(path)
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, obj)
}

func TestDockerImageCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.Resource{
		Docker: &compliance.DockerResource{
			Kind: "image",
		},
		Condition: `docker.template("{{- $.Config.Healthcheck.Test -}}") != ""`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var images []types.ImageSummary
	assert.NoError(loadTestJSON("./testdata/docker/image-list.json", &images))
	client.On("ImageList", mockCtx, types.ImageListOptions{All: true}).Return(images, nil)

	// Only iterated images here (second item stops the iteration)
	imageIDMap := map[string]string{
		"sha256:09f3f4e9394f7620fb6f1025755c85dac07f7e7aa4fca4ba19e4a03590b63750": "./testdata/docker/image-09f3f4e9394f.json",
		"sha256:f9b9909726890b00d2098081642edf32e5211b7ab53563929a47f250bcdc1d7c": "./testdata/docker/image-f9b990972689.json",
	}

	for id, path := range imageIDMap {
		var image types.ImageInspect
		assert.NoError(loadTestJSON(path, &image))
		client.On("ImageInspectWithRaw", mockCtx, id).Return(image, nil, nil)
	}

	env := &mocks.Env{}
	defer env.AssertExpectations(t)
	env.On("DockerClient").Return(client)

	expr, err := eval.ParseIterable(resource.Condition)
	assert.NoError(err)

	report, err := checkDocker(env, "rule-id", resource, expr)
	assert.NoError(err)

	assert.False(report.passed)
	assert.Equal("sha256:f9b9909726890b00d2098081642edf32e5211b7ab53563929a47f250bcdc1d7c", report.data["image.id"])
	assert.Equal([]string{"redis:latest"}, report.data["image.tags"])
}

func TestDockerNetworkCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.Resource{
		Docker: &compliance.DockerResource{
			Kind: "network",
		},
		Condition: `docker.template("{{- index $.Options \"com.docker.network.bridge.default_bridge\" -}}") != "true" || docker.template("{{- index $.Options \"com.docker.network.bridge.enable_icc\" -}}") == "true"`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var networks []types.NetworkResource
	assert.NoError(loadTestJSON("./testdata/docker/network-list.json", &networks))
	client.On("NetworkList", mockCtx, types.NetworkListOptions{}).Return(networks, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)
	env.On("DockerClient").Return(client)

	expr, err := eval.ParseIterable(resource.Condition)
	assert.NoError(err)

	report, err := checkDocker(env, "rule-id", resource, expr)
	assert.NoError(err)

	assert.True(report.passed)
	assert.Equal("bridge", report.data["network.name"])
}

func TestDockerContainerCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.Resource{
		Docker: &compliance.DockerResource{
			Kind: "container",
		},
		Condition: `docker.template("{{- $.HostConfig.Privileged -}}") != "true"`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var containers []types.Container
	assert.NoError(loadTestJSON("./testdata/docker/container-list.json", &containers))
	client.On("ContainerList", mockCtx, types.ContainerListOptions{All: true}).Return(containers, nil)

	var container types.ContainerJSON
	assert.NoError(loadTestJSON("./testdata/docker/container-3c4bd9d35d42.json", &container))
	client.On("ContainerInspect", mockCtx, "3c4bd9d35d42efb2314b636da42d4edb3882dc93ef0b1931ed0e919efdceec87").Return(container, nil, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)
	env.On("DockerClient").Return(client)

	expr, err := eval.ParseIterable(resource.Condition)
	assert.NoError(err)

	report, err := checkDocker(env, "rule-id", resource, expr)
	assert.NoError(err)

	assert.False(report.passed)
	assert.Equal("3c4bd9d35d42efb2314b636da42d4edb3882dc93ef0b1931ed0e919efdceec87", report.data["container.id"])
	assert.Equal("/sharp_cori", report.data["container.name"])
	assert.Equal("sha256:b4ceee5c3fa3cea2607d5e2bcc54d019be616e322979be8fc7a8d0d78b59a1f1", report.data["container.image"])
}

func TestDockerInfoCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.Resource{
		Docker: &compliance.DockerResource{
			Kind: "info",
		},
		Condition: `docker.template("{{- $.RegistryConfig.InsecureRegistryCIDRs | join \",\" -}}") == ""`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var info types.Info
	assert.NoError(loadTestJSON("./testdata/docker/info.json", &info))
	client.On("Info", mockCtx).Return(info, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)
	env.On("DockerClient").Return(client)

	expr, err := eval.ParseIterable(resource.Condition)
	assert.NoError(err)

	report, err := checkDocker(env, "rule-id", resource, expr)
	assert.NoError(err)

	assert.False(report.passed)
}

func TestDockerVersionCheck(t *testing.T) {
	assert := assert.New(t)

	resource := compliance.Resource{
		Docker: &compliance.DockerResource{
			Kind: "version",
		},
		Condition: `docker.template("{{ range $.Components }}{{ if eq .Name \"Engine\" }}{{- .Details.Experimental -}}{{ end }}{{ end }}") == ""`,
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var version types.Version
	assert.NoError(loadTestJSON("./testdata/docker/version.json", &version))
	client.On("ServerVersion", mockCtx).Return(version, nil)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)
	env.On("DockerClient").Return(client)

	expr, err := eval.ParseIterable(resource.Condition)
	assert.NoError(err)

	report, err := checkDocker(env, "rule-id", resource, expr)
	assert.NoError(err)

	assert.False(report.passed)
	assert.Equal("19.03.6", report.data["docker.version"])
}
