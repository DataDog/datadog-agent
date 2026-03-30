// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hosttags provides access to host tags
package hosttags

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
)

func setupTest(t *testing.T) (model.Config, context.Context) {
	retrySleepTime = 0
	origGetOSVersion := getOSVersionFunc
	origCollectSystemInfo := collectSystemInfoFn
	t.Cleanup(func() {
		retrySleepTime = 1 * time.Second
		getProvidersDefinitionsFunc = getProvidersDefinitions
		getOSVersionFunc = origGetOSVersion
		collectSystemInfoFn = origCollectSystemInfo
	})

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("autoconfig_from_environment", false)
	return mockConfig, context.Background()
}

func TestGet(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("tags", []string{"tag1:value1", "tag2", "tag3"})
	mockConfig.SetWithoutSource("tags", []string{"tag1:value1", "tag2", "tag3"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetEmptyHostTags(t *testing.T) {
	mockConfig, ctx := setupTest(t)

	// Get should never return a nil value under System even when there are no host tags
	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{}, hostTags.System)
}

func TestGetWithSplits(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("tag_value_split_separator", map[string]string{"kafka_partition": ","})
	mockConfig.SetWithoutSource("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0", "kafka_partition:1", "kafka_partition:2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetWithoutSplits(t *testing.T) {
	mockConfig, ctx := setupTest(t)

	mockConfig.SetWithoutSource("tag_value_split_separator", map[string]string{"kafka_partition": ";"})
	mockConfig.SetWithoutSource("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0,1,2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetWithEnv(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("tags", []string{"tag1:value1", "tag2", "tag3", "env:prod"})
	mockConfig.SetWithoutSource("env", "preprod")

	hostTags := Get(ctx, false, mockConfig)
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
	mockConfig.SetWithoutSource("tags", []string{"tag1:value1", "tag2", "tag4"})
	mockConfig.SetWithoutSource("extra_tags", []string{"tag1:value2", "tag3", "tag4"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag1:value2", "tag2", "tag3", "tag4"}, hostTags.System)
}

func TestHostTagsCache(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("collect_gce_tags", false)

	fooTags := []string{"foo1:value1"}
	var fooErr error
	nbCall := 0

	getProvidersDefinitionsFunc = func(model.Reader) map[string]*providerDef {
		return map[string]*providerDef{
			"foo": {
				retries: 2,
				getTags: func(_ context.Context) ([]string, error) {
					nbCall++
					return fooTags, fooErr
				},
			},
		}
	}

	// First run, all good
	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"foo1:value1"}, hostTags.System)
	assert.Equal(t, 1, nbCall)

	// Second run, provider all fails, we should get cached data
	fooErr = errors.New("fooerr")
	nbCall = 0

	hostTags = Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"foo1:value1"}, hostTags.System)
	assert.Equal(t, 2, nbCall)
}

func TestGetEUDMHostTags(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("infrastructure_mode", "end_user_device")

	getOSVersionFunc = func() string { return "Windows 10.0.19045" }
	collectSystemInfoFn = func() (*systeminfo.SystemInfo, error) {
		return &systeminfo.SystemInfo{ModelName: "ThinkPad X1 Carbon"}, nil
	}

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Contains(t, hostTags.System, "os:Windows")
	assert.Contains(t, hostTags.System, "os_version:10.0.19045")
	assert.Contains(t, hostTags.System, "device_model:ThinkPad X1 Carbon")
}

func TestGetEUDMHostTagsMacOS(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("infrastructure_mode", "end_user_device")

	getOSVersionFunc = func() string { return "macOS 14.5" }
	collectSystemInfoFn = func() (*systeminfo.SystemInfo, error) {
		return &systeminfo.SystemInfo{ModelName: "MacBook Pro"}, nil
	}

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Contains(t, hostTags.System, "os:macOS")
	assert.Contains(t, hostTags.System, "os_version:14.5")
	assert.Contains(t, hostTags.System, "device_model:MacBook Pro")
}

func TestGetEUDMHostTagsNotEnabled(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("infrastructure_mode", "full")

	getOSVersionFunc = func() string { return "Windows 10.0.19045" }
	collectSystemInfoFn = func() (*systeminfo.SystemInfo, error) {
		return &systeminfo.SystemInfo{ModelName: "ThinkPad X1 Carbon"}, nil
	}

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.NotContains(t, hostTags.System, "os:Windows")
	assert.NotContains(t, hostTags.System, "os_version:10.0.19045")
	assert.NotContains(t, hostTags.System, "device_model:ThinkPad X1 Carbon")
}

func TestGetEUDMHostTagsSystemInfoError(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("infrastructure_mode", "end_user_device")

	getOSVersionFunc = func() string { return "Windows 10.0.19045" }
	collectSystemInfoFn = func() (*systeminfo.SystemInfo, error) {
		return nil, errors.New("WMI unavailable")
	}

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Contains(t, hostTags.System, "os:Windows")
	assert.Contains(t, hostTags.System, "os_version:10.0.19045")
	assert.NotContains(t, hostTags.System, "device_model:")
}

func TestGetEUDMHostTagsOSOnlyNoVersion(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("infrastructure_mode", "end_user_device")

	getOSVersionFunc = func() string { return "Linux" }
	collectSystemInfoFn = func() (*systeminfo.SystemInfo, error) {
		return nil, nil
	}

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Contains(t, hostTags.System, "os:Linux")
}

func TestGetEUDMHostTagsEmptyModelName(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetWithoutSource("infrastructure_mode", "end_user_device")

	getOSVersionFunc = func() string { return "Windows 10.0.19045" }
	collectSystemInfoFn = func() (*systeminfo.SystemInfo, error) {
		return &systeminfo.SystemInfo{ModelName: ""}, nil
	}

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Contains(t, hostTags.System, "os:Windows")
	assert.Contains(t, hostTags.System, "os_version:10.0.19045")
	for _, tag := range hostTags.System {
		assert.NotContains(t, tag, "device_model:")
	}
}
