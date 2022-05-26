// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"os"
	"testing"

	"gotest.tools/assert"
)

func TestGetBaseTagsArrayNoEnvNoMetadata(t *testing.T) {
	assert.Equal(t, 0, len(GetBaseTagsArrayWithMetadataTags(make(map[string]string, 0))))
}

func GetBaseTagsArrayWithMetadataTagsNoMetadata(t *testing.T) {
	os.Setenv("K_SERVICE", "myService")
	defer os.Unsetenv("K_SERVICE")
	os.Setenv("K_REVISION", "FDGF34")
	defer os.Unsetenv("K_REVISION")
	os.Setenv("DD_ENV", "myEnv")
	defer os.Unsetenv("DD_ENV")
	os.Setenv("DD_SERVICE", "superService")
	defer os.Unsetenv("DD_SERVICE")
	os.Setenv("DD_VERSION", "123.4")
	defer os.Unsetenv("DD_VERSION")
	tags := GetBaseTagsArrayWithMetadataTags(make(map[string]string, 0))
	assert.Equal(t, 5, len(tags))
	assert.Equal(t, "cloudrunrevision:fdgf34", tags[0])
	assert.Equal(t, "cloudrunservice:myservice", tags[1])
	assert.Equal(t, "env:myenv", tags[2])
	assert.Equal(t, "service:superservice", tags[3])
	assert.Equal(t, "version:123.4", tags[4])
}

func TestGetTagFound(t *testing.T) {
	os.Setenv("TOTO", "coucou")
	defer os.Unsetenv("TOTO")
	value, found := getTag("TOTO")
	assert.Equal(t, true, found)
	assert.Equal(t, "coucou", value)
}

func TestGetTagNotFound(t *testing.T) {
	value, found := getTag("XXX")
	assert.Equal(t, false, found)
	assert.Equal(t, "", value)
}

func TestGetBaseTagsMapNoEnvNoMetadata(t *testing.T) {
	assert.Equal(t, 0, len(GetBaseTagsMapWithMetadata(make(map[string]string, 0))))
}

func TestGetBaseTagsMapNoMetadata(t *testing.T) {
	os.Setenv("K_SERVICE", "myService")
	defer os.Unsetenv("K_SERVICE")
	os.Setenv("K_REVISION", "FDGF34")
	defer os.Unsetenv("K_REVISION")
	os.Setenv("DD_ENV", "myEnv")
	defer os.Unsetenv("DD_ENV")
	os.Setenv("DD_SERVICE", "superService")
	defer os.Unsetenv("DD_SERVICE")
	os.Setenv("DD_VERSION", "123.4")
	defer os.Unsetenv("DD_VERSION")
	tags := GetBaseTagsMapWithMetadata(make(map[string]string, 0))
	assert.Equal(t, 5, len(tags))
	assert.Equal(t, "fdgf34", tags["cloudrunrevision"])
	assert.Equal(t, "myservice", tags["cloudrunservice"])
	assert.Equal(t, "myenv", tags["env"])
	assert.Equal(t, "superservice", tags["service"])
	assert.Equal(t, "123.4", tags["version"])
}

func TestGetBaseTagsMapWithMetadata(t *testing.T) {
	os.Setenv("K_SERVICE", "myService")
	defer os.Unsetenv("K_SERVICE")
	tags := GetBaseTagsMapWithMetadata(map[string]string{
		"region":        "mysuperregion",
		"othermetadata": "mysuperothermetadatavalue",
	})
	assert.Equal(t, 3, len(tags))
	assert.Equal(t, "myservice", tags["cloudrunservice"])
	assert.Equal(t, "mysuperregion", tags["region"])
	assert.Equal(t, "mysuperothermetadatavalue", tags["othermetadata"])
}

func TestGetBaseTagsArrayWithMetadataTags(t *testing.T) {
	os.Setenv("K_REVISION", "FDGF34")
	defer os.Unsetenv("K_REVISION")
	tags := GetBaseTagsArrayWithMetadataTags(map[string]string{
		"region":        "mysuperregion",
		"othermetadata": "mysuperothermetadatavalue",
	})
	assert.Equal(t, 3, len(tags))
	assert.Equal(t, "cloudrunrevision:fdgf34", tags[0])
	assert.Equal(t, "region:mysuperregion", tags[1])
	assert.Equal(t, "othermetadata:mysuperothermetadatavalue", tags[2])
}
