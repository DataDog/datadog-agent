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
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/docker/docker/api/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	mockCtx       = mock.Anything
	testCheckMeta = struct {
		framework    string
		version      string
		ruleID       string
		resourceID   string
		resourceType string
	}{
		framework:    "cis-docker",
		version:      "1.2.0",
		ruleID:       "rule",
		resourceID:   "host",
		resourceType: "docker",
	}
)

func newTestRuleEvent(tags []string, kv compliance.KV) *compliance.RuleEvent {
	return &compliance.RuleEvent{
		RuleID:       testCheckMeta.ruleID,
		Framework:    testCheckMeta.framework,
		Version:      testCheckMeta.version,
		ResourceType: testCheckMeta.resourceType,
		ResourceID:   testCheckMeta.resourceID,
		Tags:         tags,
		Data:         kv,
	}
}

func newTestBaseCheck(reporter compliance.Reporter, kind checkKind) baseCheck {
	return baseCheck{
		id:        check.ID("check-1"),
		kind:      kind,
		interval:  time.Minute,
		framework: testCheckMeta.framework,
		version:   testCheckMeta.version,

		ruleID:       testCheckMeta.ruleID,
		resourceType: testCheckMeta.resourceType,
		resourceID:   testCheckMeta.resourceID,
		reporter:     reporter,
	}
}

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

	resource := &compliance.DockerResource{
		Kind: "image",

		Filter: []compliance.Filter{
			{
				Exclude: &compliance.Condition{
					Property:  "{{- $.Config.Healthcheck.Test -}}",
					Operation: compliance.OpExists,
				},
			},
		},
		Report: compliance.Report{
			{
				Property: "id",
				As:       "image_id",
			},
			{
				Value: "true",
				As:    "image_healthcheck_missing",
			},
			{
				Property: "{{- index $.RepoTags 0 -}}",
				Kind:     compliance.PropertyKindTemplate,
				As:       "image_name",
			},
		},
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var images []types.ImageSummary
	assert.NoError(loadTestJSON("./testdata/docker/image-list.json", &images))
	client.On("ImageList", mockCtx, types.ImageListOptions{All: true}).Return(images, nil)

	imageIDMap := map[string]string{
		"sha256:09f3f4e9394f7620fb6f1025755c85dac07f7e7aa4fca4ba19e4a03590b63750": "./testdata/docker/image-09f3f4e9394f.json",
		"sha256:89ec9da682137d6b18ab8244ca263b6771067f251562f884c7510c8f1e5ac910": "./testdata/docker/image-89ec9da68213.json",
		"sha256:f9b9909726890b00d2098081642edf32e5211b7ab53563929a47f250bcdc1d7c": "./testdata/docker/image-f9b990972689.json",
	}

	for id, path := range imageIDMap {
		var image types.ImageInspect
		assert.NoError(loadTestJSON(path, &image))
		client.On("ImageInspectWithRaw", mockCtx, id).Return(image, nil, nil)
	}

	reporter := &mocks.Reporter{}
	defer reporter.AssertExpectations(t)

	imagesWithMissingHealthcheck := []struct {
		id   string
		name string
	}{
		{
			id:   "sha256:f9b9909726890b00d2098081642edf32e5211b7ab53563929a47f250bcdc1d7c",
			name: "redis:latest",
		},
		{
			id:   "sha256:89ec9da682137d6b18ab8244ca263b6771067f251562f884c7510c8f1e5ac910",
			name: "nginx:alpine",
		},
	}

	for _, image := range imagesWithMissingHealthcheck {
		reporter.On(
			"Report",
			newTestRuleEvent(
				[]string{"check_kind:docker"},
				compliance.KV{
					"image_id":                  image.id,
					"image_name":                image.name,
					"image_healthcheck_missing": "true",
				},
			),
		).Once()
	}

	dockerCheck := dockerCheck{
		baseCheck:      newTestBaseCheck(reporter, checkKindDocker),
		client:         client,
		dockerResource: resource,
	}

	err := dockerCheck.Run()
	assert.NoError(err)
}

func TestDockerNetworkCheck(t *testing.T) {
	assert := assert.New(t)

	resource := &compliance.DockerResource{
		Kind: "network",

		Filter: []compliance.Filter{
			{
				Include: &compliance.Condition{
					Property:  `{{- index $.Options "com.docker.network.bridge.default_bridge" -}}`,
					Kind:      compliance.PropertyKindTemplate,
					Operation: compliance.OpEqual,
					Value:     "true",
				},
			},
		},
		Report: compliance.Report{
			{
				Property: "id",
				As:       "network_id",
			},
			{
				Property: `{{- index $.Options "com.docker.network.bridge.enable_icc" -}}`,
				Kind:     compliance.PropertyKindTemplate,
				As:       "default_bridge_traffic_restricted",
			},
		},
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var networks []types.NetworkResource
	assert.NoError(loadTestJSON("./testdata/docker/network-list.json", &networks))
	client.On("NetworkList", mockCtx, types.NetworkListOptions{}).Return(networks, nil)

	reporter := &mocks.Reporter{}
	defer reporter.AssertExpectations(t)

	reporter.On(
		"Report",
		newTestRuleEvent(
			[]string{"check_kind:docker"},
			compliance.KV{
				"network_id":                        "e7ed6c335383178f99b61a8a44b82b62abc17b31d68b792180728bf8f2c599ec",
				"default_bridge_traffic_restricted": "true",
			},
		),
	).Once()

	dockerCheck := dockerCheck{
		baseCheck:      newTestBaseCheck(reporter, checkKindDocker),
		client:         client,
		dockerResource: resource,
	}

	err := dockerCheck.Run()
	assert.NoError(err)
}

func TestDockerContainerCheck(t *testing.T) {
	assert := assert.New(t)

	resource := &compliance.DockerResource{
		Kind: "container",

		Filter: []compliance.Filter{
			{
				Include: &compliance.Condition{
					Property:  `{{- $.HostConfig.Privileged -}}`,
					Kind:      compliance.PropertyKindTemplate,
					Operation: compliance.OpEqual,
					Value:     "true",
				},
			},
		},
		Report: compliance.Report{
			{
				Property: "id",
				As:       "container_id",
			},
			{
				As:    "privileged",
				Value: "true",
			},
		},
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var containers []types.Container
	assert.NoError(loadTestJSON("./testdata/docker/container-list.json", &containers))
	client.On("ContainerList", mockCtx, types.ContainerListOptions{All: true}).Return(containers, nil)

	var container types.ContainerJSON
	assert.NoError(loadTestJSON("./testdata/docker/container-3c4bd9d35d42.json", &container))
	client.On("ContainerInspect", mockCtx, "3c4bd9d35d42efb2314b636da42d4edb3882dc93ef0b1931ed0e919efdceec87").Return(container, nil, nil)

	reporter := &mocks.Reporter{}
	defer reporter.AssertExpectations(t)

	reporter.On(
		"Report",
		newTestRuleEvent(
			[]string{"check_kind:docker"},
			compliance.KV{
				"container_id": "3c4bd9d35d42efb2314b636da42d4edb3882dc93ef0b1931ed0e919efdceec87",
				"privileged":   "true",
			},
		),
	).Once()

	dockerCheck := dockerCheck{
		baseCheck:      newTestBaseCheck(reporter, checkKindDocker),
		client:         client,
		dockerResource: resource,
	}

	err := dockerCheck.Run()
	assert.NoError(err)
}

func TestDockerInfoCheck(t *testing.T) {
	assert := assert.New(t)

	resource := &compliance.DockerResource{
		Kind: "info",
		Report: compliance.Report{
			{
				Property: `{{- $.RegistryConfig.InsecureRegistryCIDRs | join "," -}}`,
				Kind:     compliance.PropertyKindTemplate,
				As:       "insecure_registries",
			},
		},
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var info types.Info
	assert.NoError(loadTestJSON("./testdata/docker/info.json", &info))
	client.On("Info", mockCtx).Return(info, nil)

	reporter := &mocks.Reporter{}
	defer reporter.AssertExpectations(t)

	reporter.On(
		"Report",
		newTestRuleEvent(
			[]string{"check_kind:docker"},
			compliance.KV{
				"insecure_registries": "127.0.0.0/8",
			},
		),
	).Once()

	dockerCheck := dockerCheck{
		baseCheck:      newTestBaseCheck(reporter, checkKindDocker),
		client:         client,
		dockerResource: resource,
	}

	err := dockerCheck.Run()
	assert.NoError(err)
}

func TestDockerVersionCheck(t *testing.T) {
	assert := assert.New(t)

	resource := &compliance.DockerResource{
		Kind: "version",
		Report: compliance.Report{
			{
				Property: `{{ range $.Components }}{{ if eq .Name "Engine" }}{{- .Details.Experimental -}}{{ end }}{{ end }}`,
				Kind:     compliance.PropertyKindTemplate,
				As:       "experimental_features",
			},
		},
	}

	client := &mocks.DockerClient{}
	defer client.AssertExpectations(t)

	var version types.Version
	assert.NoError(loadTestJSON("./testdata/docker/version.json", &version))
	client.On("ServerVersion", mockCtx).Return(version, nil)

	reporter := &mocks.Reporter{}
	defer reporter.AssertExpectations(t)

	reporter.On(
		"Report",
		newTestRuleEvent(
			[]string{"check_kind:docker"},
			compliance.KV{
				"experimental_features": "true",
			},
		),
	).Once()

	dockerCheck := dockerCheck{
		baseCheck:      newTestBaseCheck(reporter, checkKindDocker),
		client:         client,
		dockerResource: resource,
	}

	err := dockerCheck.Run()
	assert.NoError(err)
}
