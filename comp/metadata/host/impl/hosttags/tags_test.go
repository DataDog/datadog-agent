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
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func setupTest(t *testing.T) (model.Config, context.Context) {
	retrySleepTime = 0
	t.Cleanup(func() {
		retrySleepTime = 1 * time.Second
		getProvidersDefinitionsFunc = getProvidersDefinitions
	})

	mockConfig := configmock.New(t)
	mockConfig.SetInTest("autoconfig_from_environment", false)
	return mockConfig, context.Background()
}

func TestGet(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3"})
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3"})

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
	mockConfig.SetInTest("tag_value_split_separator", map[string]string{"kafka_partition": ","})
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0", "kafka_partition:1", "kafka_partition:2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetWithoutSplits(t *testing.T) {
	mockConfig, ctx := setupTest(t)

	mockConfig.SetInTest("tag_value_split_separator", map[string]string{"kafka_partition": ";"})
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0,1,2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetWithEnv(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3", "env:prod"})
	mockConfig.SetInTest("env", "preprod")

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
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag4"})
	mockConfig.SetInTest("extra_tags", []string{"tag1:value2", "tag3", "tag4"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag1:value2", "tag2", "tag3", "tag4"}, hostTags.System)
}

func TestGetWithoutEUDM(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("infrastructure_mode", "full")

	hostTags := Get(ctx, false, mockConfig)
	for _, tag := range hostTags.System {
		assert.NotContains(t, tag, "infra_mode:")
		assert.NotContains(t, tag, "os_name:")
		assert.NotContains(t, tag, "os_version:")
		assert.NotContains(t, tag, "cpu_model:")
		assert.NotContains(t, tag, "device_model:")
		assert.NotContains(t, tag, "total_memory_gb:")
	}
}

func TestGetWithEUDM(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("infrastructure_mode", "end_user_device")

	original := collectEUDMTagsFunc
	t.Cleanup(func() { collectEUDMTagsFunc = original })
	collectEUDMTagsFunc = func() []string {
		return []string{
			"os_name:darwin",
			"os_version:23.5.0",
			"cpu_model:Apple_M1_Pro",
			"total_memory_gb:16",
			"device_model:MacBookPro18,3",
		}
	}

	hostTags := Get(ctx, false, mockConfig)
	assert.Contains(t, hostTags.System, "infra_mode:end_user_device")
	assert.Contains(t, hostTags.System, "os_name:darwin")
	assert.Contains(t, hostTags.System, "os_version:23.5.0")
	assert.Contains(t, hostTags.System, "cpu_model:Apple_M1_Pro")
	assert.Contains(t, hostTags.System, "total_memory_gb:16")
	assert.Contains(t, hostTags.System, "device_model:MacBookPro18,3")
}

func TestEUDMTagsOnUnsupportedOS(t *testing.T) {
	// collectEUDMHardwareTags should return nil on non-darwin/windows so the
	// only EUDM tag emitted on Linux is the infra_mode marker.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("test asserts behavior on non-darwin/non-windows hosts")
	}
	assert.Nil(t, collectEUDMHardwareTags())

	tags := getEUDMTags()
	assert.Equal(t, []string{"infra_mode:end_user_device"}, tags)
}

func TestBytesToGB(t *testing.T) {
	assert.Equal(t, uint64(16), bytesToGB(16*1024*1024*1024))
	assert.Equal(t, uint64(0), bytesToGB(0))
	assert.Equal(t, uint64(1), bytesToGB(1024*1024*1024))
	// 15.9 GiB rounds to 16
	assert.Equal(t, uint64(16), bytesToGB(15*1024*1024*1024+900*1024*1024))
}

func TestSanitizeEUDMTagValue(t *testing.T) {
	assert.Equal(t, "Apple_M1_Pro", sanitizeEUDMTagValue("Apple M1 Pro"))
	assert.Equal(t, "MacBookPro18,3", sanitizeEUDMTagValue("MacBookPro18,3"))
	assert.Equal(t, "trim_me", sanitizeEUDMTagValue("  trim me  "))
}

func TestHostTagsCache(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("collect_gce_tags", false)

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
