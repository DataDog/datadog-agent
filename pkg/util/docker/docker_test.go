// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
)

func TestResolveImageName(t *testing.T) {
	ctx := context.Background()
	imageName := "datadog/docker-dd-agent:latest"
	imageSha := "sha256:bdc7dc8ba08c2ac8c8e03550d8ebf3297a669a3f03e36c377b9515f08c1b4ef4"
	imageWithShaTag := "datadog/docker-dd-agent@sha256:9aab42bf6a2a068b797fe7d91a5d8d915b10dbbc3d6f2b10492848debfba6044"

	assert := assert.New(t)
	globalDockerUtil = &DockerUtil{
		cfg:            &Config{CollectNetwork: false},
		cli:            nil,
		imageNameBySha: make(map[string]string),
	}
	globalDockerUtil.imageNameBySha[imageWithShaTag] = imageName
	globalDockerUtil.imageNameBySha[imageSha] = imageName
	for i, tc := range []struct {
		input    string
		expected string
	}{
		{
			input:    "",
			expected: "",
		}, {
			input:    imageName,
			expected: imageName,
		}, {
			input:    imageWithShaTag,
			expected: imageName,
		}, {
			input:    imageSha,
			expected: imageName,
		},
	} {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			name, err := globalDockerUtil.ResolveImageName(ctx, tc.input)
			assert.Equal(tc.expected, name, "test %s failed", i)
			assert.Nil(err, "test %s failed", i)
		})
	}
}

func TestResolveImageNameFromContainer(t *testing.T) {
	ctx := context.Background()
	imageName := "datadog/docker-dd-agent:latest"
	imageSha := "sha256:bdc7dc8ba08c2ac8c8e03550d8ebf3297a669a3f03e36c377b9515f08c1b4ef4"
	imageWithShaTag := "datadog/docker-dd-agent@sha256:9aab42bf6a2a068b797fe7d91a5d8d915b10dbbc3d6f2b10492848debfba6044"

	assert := assert.New(t)
	globalDockerUtil = &DockerUtil{
		cfg:            &Config{CollectNetwork: false},
		cli:            nil,
		imageNameBySha: make(map[string]string),
	}
	globalDockerUtil.imageNameBySha[imageWithShaTag] = imageName
	globalDockerUtil.imageNameBySha[imageSha] = imageName

	for _, tc := range []struct {
		name          string
		input         types.ContainerJSON
		expectedImage string
	}{
		{
			name: "test empty config image name",
			input: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{Image: imageSha},
				Config:            &container.Config{},
			},
			expectedImage: imageName,
		},
		{
			name: "test standard config image name",
			input: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{Image: "ignored"},
				Config:            &container.Config{Image: imageName},
			},
			expectedImage: imageName,
		},
		{
			name: "test config image name as sha tag",
			input: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{Image: imageSha},
				Config:            &container.Config{Image: imageSha},
			},
			expectedImage: imageName,
		},
		{
			name: "test config image name with sha tag",
			input: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{Image: imageSha},
				Config:            &container.Config{Image: imageWithShaTag},
			},
			expectedImage: imageName,
		},
	} {
		t.Run(fmt.Sprintf("case %s", tc.name), func(t *testing.T) {
			result, err := globalDockerUtil.ResolveImageNameFromContainer(ctx, tc.input)
			assert.Equal(tc.expectedImage, result, "%s test failed; expected %s but got %s", tc.name, tc.expectedImage, result)
			assert.Nil(err, "%s test failed; expected nil error but got %s", tc.name, err)
		})
	}
}

func TestResolveImageNameFromContainerError(t *testing.T) {
	ctx := context.Background()
	imageSha := "sha256:bdc7dc8ba08c2ac8c8e03550d8ebf3297a669a3f03e36c377b9515f08c1b4ef4"
	assert := assert.New(t)

	// This returns a nil client because the transport verification fails
	cli, _ := client.NewClientWithOpts(client.FromEnv)

	globalDockerUtil = &DockerUtil{
		cfg:            &Config{CollectNetwork: false},
		cli:            cli,
		imageNameBySha: make(map[string]string),
	}

	input := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{Image: imageSha},
		Config:            &container.Config{Image: imageSha},
	}

	result, err := globalDockerUtil.ResolveImageNameFromContainer(ctx, input)
	assert.Equal(imageSha, result, "test failed; expected %s but got %s", imageSha, result)
	assert.NotNil(err, "test failed; expected an error but got %s", err)
}
