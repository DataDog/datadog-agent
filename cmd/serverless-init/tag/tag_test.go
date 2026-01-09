// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

func TestGetBaseTagsArrayNoEnvNoMetadata(t *testing.T) {
	assert.Equal(t, 1, len(GetBaseTagsMapWithMetadata(make(map[string]string, 0), "")))
}

func TestGetBaseTagsArrayWithMetadataTagsNoMetadata(t *testing.T) {
	t.Setenv("K_SERVICE", "myService")
	t.Setenv("K_REVISION", "FDGF34")
	t.Setenv("DD_ENV", "myEnv")
	t.Setenv("DD_SERVICE", "superService")
	t.Setenv("DD_VERSION", "123.4")
	tags := serverlessTag.MapToArray(GetBaseTagsMapWithMetadata(make(map[string]string, 0), "_dd.datadog_init_version"))
	sort.Strings(tags)
	assert.Equal(t, 4, len(tags))
	assert.Contains(t, tags[0], "_dd.datadog_init_version")
	assert.Equal(t, "env:myenv", tags[1])
	assert.Equal(t, "service:superservice", tags[2])
	assert.Equal(t, "version:123.4", tags[3])
}

func TestGetTagFound(t *testing.T) {
	t.Setenv("TOTO", "coucou")
	value, found := getTagFromEnv("TOTO")
	assert.Equal(t, true, found)
	assert.Equal(t, "coucou", value)
}

func TestGetTagNotFound(t *testing.T) {
	value, found := getTagFromEnv("XXX")
	assert.Equal(t, false, found)
	assert.Equal(t, "", value)
}

func TestGetBaseTagsMapNoEnvNoMetadata(t *testing.T) {
	assert.Equal(t, 1, len(GetBaseTagsMapWithMetadata(make(map[string]string, 0), "")))
}

func TestGetBaseTagsMapNoMetadata(t *testing.T) {
	t.Setenv("K_SERVICE", "myService")
	t.Setenv("K_REVISION", "FDGF34")
	t.Setenv("DD_ENV", "myEnv")
	t.Setenv("DD_SERVICE", "superService")
	t.Setenv("DD_VERSION", "123.4")
	tags := GetBaseTagsMapWithMetadata(make(map[string]string, 0), "_dd.version")
	assert.Equal(t, 4, len(tags))
	assert.Equal(t, "myenv", tags["env"])
	assert.Equal(t, "superservice", tags["service"])
	assert.Equal(t, "123.4", tags["version"])
}

func TestGetBaseTagsMapWithMetadata(t *testing.T) {
	t.Setenv("K_SERVICE", "myService")
	tags := GetBaseTagsMapWithMetadata(map[string]string{
		"location":      "mysuperlocation",
		"othermetadata": "mysuperothermetadatavalue",
	}, "_dd.version")
	assert.Equal(t, 3, len(tags))
	assert.Equal(t, "mysuperlocation", tags["location"])
	assert.Equal(t, "mysuperothermetadatavalue", tags["othermetadata"])
}

func TestGetBaseTagsArrayWithMetadataTags(t *testing.T) {
	t.Setenv("K_REVISION", "FDGF34")
	tags := serverlessTag.MapToArray(GetBaseTagsMapWithMetadata(map[string]string{
		"location":      "mysuperlocation",
		"othermetadata": "mysuperothermetadatavalue",
	}, "_dd.datadog_sidecar_version"))
	sort.Strings(tags)
	assert.Equal(t, 3, len(tags))
	assert.Contains(t, tags[0], "_dd.datadog_sidecar_version")
	assert.Equal(t, "location:mysuperlocation", tags[1])
	assert.Equal(t, "othermetadata:mysuperothermetadatavalue", tags[2])
}

func TestDdTags(t *testing.T) {
	t.Setenv("DD_TAGS", "originalKey:shouldNotOverride key2:value2 key3:value3")
	t.Setenv("DD_EXTRA_TAGS", "key5:value5 key6:value6")
	overwritingTags := map[string]string{
		"originalKey": "overWrittenValue",
	}
	cfg := mock.New(t)
	mergedTags := serverlessTag.MergeWithOverwrite(serverlessTag.ArrayToMap(configUtils.GetConfiguredTags(cfg, false)), overwritingTags)
	assert.Equal(t, "overWrittenValue", mergedTags["originalKey"])
	assert.Equal(t, "value2", mergedTags["key2"])
	assert.Equal(t, "value3", mergedTags["key3"])
	assert.Equal(t, "value5", mergedTags["key5"])
	assert.Equal(t, "value6", mergedTags["key6"])
}

func TestMakeMetricAgentTags(t *testing.T) {
	tags := map[string]string{
		"key1":                "value1",
		"key2":                "value2",
		"container_id":        "abc",
		"replica_name":        "abc",
		"gcrj.execution_name": "exec-123",
		"gcrj.task_index":     "0",
		"gcrj.task_attempt":   "1",
		"gcrj.task_count":     "10",
	}
	filteredTags := MakeMetricAgentTags(tags)
	assert.Equal(t, map[string]string{"key1": "value1", "key2": "value2"}, filteredTags)
}

func TestMakeTraceAgentTags(t *testing.T) {
	tests := []struct {
		name                  string
		envValue              string
		expectComputeStatsTag bool
	}{
		{
			name:                  "disabled by default",
			envValue:              "",
			expectComputeStatsTag: false,
		},
		{
			name:                  "enabled with true",
			envValue:              "true",
			expectComputeStatsTag: true,
		},
		{
			name:                  "disabled with false",
			envValue:              "false",
			expectComputeStatsTag: false,
		},
		{
			name:                  "disabled with other value",
			envValue:              "yes",
			expectComputeStatsTag: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(enableBackendTraceStatsEnvVar, tt.envValue)
			}

			tags := MakeTraceAgentTags(make(map[string]string, 0))

			if tt.expectComputeStatsTag {
				// compute_stats should be present in modified tags
				assert.Equal(t, serverlessTag.ComputeStatsValue, tags[serverlessTag.ComputeStatsKey])
			} else {
				_, hasComputeStats := tags[serverlessTag.ComputeStatsKey]
				assert.False(t, hasComputeStats, "compute_stats should not be present")
			}
		})
	}
}
