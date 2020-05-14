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

func newTestBaseCheck(reporter compliance.Reporter) baseCheck {
	return baseCheck{
		id:        check.ID("check-1"),
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

		Filter: []compliance.DockerFilter{
			{
				Exclude: &compliance.DockerCondition{
					Exists: "{{- $.Config.Healthcheck.Test -}}",
				},
			},
		},
		Report: compliance.Report{
			{
				Attribute: "id",
				As:        "image_id",
			},
			{
				Value: "true",
				As:    "image_healthcheck_missing",
			},
		},
	}

	client := &MockDockerClient{}
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

	reporter := &compliance.MockReporter{}
	defer reporter.AssertExpectations(t)

	imagesWithMissingHealthcheck := []string{
		"sha256:f9b9909726890b00d2098081642edf32e5211b7ab53563929a47f250bcdc1d7c",
		"sha256:89ec9da682137d6b18ab8244ca263b6771067f251562f884c7510c8f1e5ac910",
	}

	for _, id := range imagesWithMissingHealthcheck {
		reporter.On(
			"Report",
			newTestRuleEvent(
				nil,
				compliance.KV{
					"image_id":                  id,
					"image_healthcheck_missing": "true",
				},
			),
		).Once()
	}

	dockerCheck := dockerCheck{
		baseCheck:      newTestBaseCheck(reporter),
		client:         client,
		dockerResource: resource,
	}

	err := dockerCheck.Run()
	assert.NoError(err)
}
