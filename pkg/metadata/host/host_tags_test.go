// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func init() {
	retrySleepTime = 0
}

func TestGetHostTags(t *testing.T) {
	ctx := context.Background()
	mockConfig := config.Mock(t)
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3"})
	defer mockConfig.Set("tags", nil)

	hostTags := GetHostTags(ctx, false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetEmptyHostTags(t *testing.T) {
	ctx := context.Background()
	// getHostTags should never return a nil value under System even when there are no host tags
	hostTags := GetHostTags(ctx, false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{}, hostTags.System)
}

func TestGetHostTagsWithSplits(t *testing.T) {
	ctx := context.Background()
	mockConfig := config.Mock(t)
	mockConfig.Set("tag_value_split_separator", map[string]string{"kafka_partition": ","})
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})
	defer mockConfig.Set("tags", nil)

	hostTags := GetHostTags(ctx, false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0", "kafka_partition:1", "kafka_partition:2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetHostTagsWithoutSplits(t *testing.T) {
	ctx := context.Background()
	mockConfig := config.Mock(t)
	mockConfig.Set("tag_value_split_separator", map[string]string{"kafka_partition": ";"})
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})
	defer mockConfig.Set("tags", nil)

	hostTags := GetHostTags(ctx, false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0,1,2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetHostTagsWithEnv(t *testing.T) {
	ctx := context.Background()
	mockConfig := config.Mock(t)
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag3", "env:prod"})
	mockConfig.Set("env", "preprod")
	defer mockConfig.Set("tags", nil)
	defer mockConfig.Set("env", "")

	hostTags := GetHostTags(ctx, false)
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
	ctx := context.Background()
	mockConfig := config.Mock(t)
	mockConfig.Set("tags", []string{"tag1:value1", "tag2", "tag4"})
	mockConfig.Set("extra_tags", []string{"tag1:value2", "tag3", "tag4"})
	defer mockConfig.Set("tags", nil)
	defer mockConfig.Set("extra_tags", nil)

	hostTags := GetHostTags(ctx, false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag1:value2", "tag2", "tag3", "tag4"}, hostTags.System)
}

func TestHostTagsCache(t *testing.T) {
	ctx := context.Background()
	mockConfig := config.Mock(t)
	mockConfig.Set("collect_gce_tags", false)

	fooTags := []string{"foo1:value1"}
	var fooErr error

	getProvidersDefinitionsFunc = func() map[string]*providerDef {
		return map[string]*providerDef{
			"foo": {
				retries: 1,
				getTags: func(ctx context.Context) ([]string, error) {
					return fooTags, fooErr
				},
			},
		}
	}
	defer func() {
		getProvidersDefinitionsFunc = getProvidersDefinitions
	}()

	// First run, all good
	hostTags := GetHostTags(ctx, false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"foo1:value1"}, hostTags.System)

	// Second run, provider all fails, we should get cached data
	fooErr = errors.New("fooerr")

	hostTags = GetHostTags(ctx, false)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"foo1:value1"}, hostTags.System)
}
