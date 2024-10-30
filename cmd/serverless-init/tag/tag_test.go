// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

func TestGetBaseTagsArrayNoEnvNoMetadata(t *testing.T) {
	assert.Equal(t, 2, len(GetBaseTagsMapWithMetadata(make(map[string]string, 0), "")))
}

func TestGetBaseTagsArrayWithMetadataTagsNoMetadata(t *testing.T) {
	t.Setenv("K_SERVICE", "myService")
	t.Setenv("K_REVISION", "FDGF34")
	t.Setenv("DD_ENV", "myEnv")
	t.Setenv("DD_SERVICE", "superService")
	t.Setenv("DD_VERSION", "123.4")
	tags := serverlessTag.MapToArray(GetBaseTagsMapWithMetadata(make(map[string]string, 0), "datadog_init_version"))
	sort.Strings(tags)
	assert.Equal(t, 5, len(tags))
	assert.Contains(t, tags[0], "_dd.compute_stats:1")
	assert.Contains(t, tags[1], "datadog_init_version")
	assert.Equal(t, "env:myenv", tags[2])
	assert.Equal(t, "service:superservice", tags[3])
	assert.Equal(t, "version:123.4", tags[4])
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
	assert.Equal(t, 2, len(GetBaseTagsMapWithMetadata(make(map[string]string, 0), "")))
}

func TestGetBaseTagsMapNoMetadata(t *testing.T) {
	t.Setenv("K_SERVICE", "myService")
	t.Setenv("K_REVISION", "FDGF34")
	t.Setenv("DD_ENV", "myEnv")
	t.Setenv("DD_SERVICE", "superService")
	t.Setenv("DD_VERSION", "123.4")
	tags := GetBaseTagsMapWithMetadata(make(map[string]string, 0), "")
	assert.Equal(t, 5, len(tags))
	assert.Equal(t, "myenv", tags["env"])
	assert.Equal(t, "superservice", tags["service"])
	assert.Equal(t, "123.4", tags["version"])
}

func TestGetBaseTagsMapWithMetadata(t *testing.T) {
	t.Setenv("K_SERVICE", "myService")
	tags := GetBaseTagsMapWithMetadata(map[string]string{
		"location":      "mysuperlocation",
		"othermetadata": "mysuperothermetadatavalue",
	}, "")
	assert.Equal(t, 4, len(tags))
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
	assert.Equal(t, 4, len(tags))
	assert.Contains(t, tags[0], "_dd.compute_stats:1")
	assert.Contains(t, tags[1], "_dd.datadog_sidecar_version")
	assert.Equal(t, "location:mysuperlocation", tags[2])
	assert.Equal(t, "othermetadata:mysuperothermetadatavalue", tags[3])
}

func TestDdTags(t *testing.T) {
	t.Setenv("DD_TAGS", "originalKey:shouldNotOverride key2:value2 key3:value3")
	t.Setenv("DD_EXTRA_TAGS", "key5:value5 key6:value6")
	overwritingTags := map[string]string{
		"originalKey": "overWrittenValue",
	}
	mergedTags := serverlessTag.MergeWithOverwrite(serverlessTag.ArrayToMap(configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), false)), overwritingTags)
	assert.Equal(t, "overWrittenValue", mergedTags["originalKey"])
	assert.Equal(t, "value2", mergedTags["key2"])
	assert.Equal(t, "value3", mergedTags["key3"])
	assert.Equal(t, "value5", mergedTags["key5"])
	assert.Equal(t, "value6", mergedTags["key6"])
}
