// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func setupTest(t *testing.T) (*config.MockConfig, context.Context) {
	retrySleepTime = 0
	t.Cleanup(func() {
		retrySleepTime = 1 * time.Second
		getProvidersDefinitionsFunc = getProvidersDefinitions
	})

	mockConfig := config.Mock(t)
	mockConfig.Set("autoconfig_from_environment", false)
	return mockConfig, context.Background()
}

func TestGetHostTags(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3"})
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3"})

	hostTags := GetHostTags(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetEmptyHostTags(t *testing.T) {
	mockConfig, ctx := setupTest(t)

	// getHostTags should never return a nil value under System even when there are no host tags
	hostTags := GetHostTags(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{}, hostTags.System)
}

func TestGetHostTagsWithSplits(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.Set("tag_value_split_separator", map[string]string{"kafka_partition": ","})
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})

	hostTags := GetHostTags(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0", "kafka_partition:1", "kafka_partition:2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetHostTagsWithoutSplits(t *testing.T) {
	mockConfig, ctx := setupTest(t)

	mockConfig.Set("tag_value_split_separator", map[string]string{"kafka_partition": ";"})
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})

	hostTags := GetHostTags(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0,1,2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetHostTagsWithEnv(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "env:prod"})
	mockConfig.Set("env", "preprod")

	hostTags := GetHostTags(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"env:preprod", "env:prod", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestMarshalEmptyHostTags(t *testing.T) {
	tags := &Tags{
		System:              []string{},
		GoogleCloudPlatform: []string{},
	}

	marshaled, _ := json.Marshal(tags)
	// `System` should be marshaled as an empty list
	assert.Equal(t, string(marshaled), `{"system":[]}`)
}

func TestCombineExtraTags(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag4"})
	mockConfig.Set("extra_tags", []string{"tag1:value2", "tag3", "tag4"})

	hostTags := GetHostTags(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag1:value2", "tag2", "tag3", "tag4"}, hostTags.System)
}

func TestHostTagsCache(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.Set("collect_gce_tags", false)

	fooTags := []string{"foo1:value1"}
	var fooErr error
	nbCall := 0

	getProvidersDefinitionsFunc = func(config.ConfigReader) map[string]*providerDef {
		return map[string]*providerDef{
			"foo": {
				retries: 2,
				getTags: func(ctx context.Context) ([]string, error) {
					nbCall++
					return fooTags, fooErr
				},
			},
		}
	}

	// First run, all good
	hostTags := GetHostTags(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"foo1:value1"}, hostTags.System)
	assert.Equal(t, 1, nbCall)

	// Second run, provider all fails, we should get cached data
	fooErr = errors.New("fooerr")
	nbCall = 0

	hostTags = GetHostTags(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"foo1:value1"}, hostTags.System)
	assert.Equal(t, 2, nbCall)
}
