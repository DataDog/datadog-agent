// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCloudRunTags(t *testing.T) {
	service := &CloudRun{}

	metadataHelperFunc := &traceutil.GCPMetadata{
		ContainerID: &traceutil.Info{
			TagName: "container_id",
			Value:   "test_container",
		},
		Region: &traceutil.Info{
			TagName: "region",
			Value:   "test_region",
		},
		ProjectID: &traceutil.Info{
			TagName: "project_id",
			Value:   "test_project",
		},
	}
	tags := metadataHelperFunc.TagMap()
	assert.Equal(t, map[string]string{
		"container_id": "test_container",
		"region":       "test_region",
		"project_id":   "test_project",
	}, tags)

	tags = service.GetTags()

	assert.Equal(t, map[string]string{
		"container_id": "unknown",
		"location":     "unknown",
		"origin":       "cloudrun",
		"project_id":   "unknown",
		"_dd.origin":   "cloudrun",
	}, tags)
}

func TestGetCloudRunTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRun{}

	metadataHelperFunc := &traceutil.GCPMetadata{
		ContainerID: &traceutil.Info{
			TagName: "container_id",
			Value:   "test_container",
		},
		Region: &traceutil.Info{
			TagName: "region",
			Value:   "test_region",
		},
		ProjectID: &traceutil.Info{
			TagName: "project_id",
			Value:   "test_project",
		},
	}

	tags := metadataHelperFunc.TagMap()
	assert.Equal(t, map[string]string{
		"container_id": "test_container",
		"region":       "test_region",
		"project_id":   "test_project",
	}, tags)

	t.Setenv("K_SERVICE", "test_service")
	t.Setenv("K_REVISION", "test_revision")

	tags = service.GetTags()
	assert.Equal(t, map[string]string{
		"container_id":  "unknown",
		"location":      "unknown",
		"origin":        "cloudrun",
		"project_id":    "unknown",
		"service_name":  "test_service",
		"revision_name": "test_revision",
		"_dd.origin":    "cloudrun",
	}, tags)
}

func TestGetCloudRunFunctionTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRun{}

	metadataHelperFunc := &traceutil.GCPMetadata{
		ContainerID: &traceutil.Info{
			TagName: "container_id",
			Value:   "test_container",
		},
		Region: &traceutil.Info{
			TagName: "region",
			Value:   "test_region",
		},
		ProjectID: &traceutil.Info{
			TagName: "project_id",
			Value:   "test_project",
		},
	}

	tags := metadataHelperFunc.TagMap()
	assert.Equal(t, map[string]string{
		"container_id": "test_container",
		"region":       "test_region",
		"project_id":   "test_project",
	}, tags)

	t.Setenv("K_SERVICE", "test_service")
	t.Setenv("K_REVISION", "test_revision")
	t.Setenv("K_CONFIGURATION", "test_config")
	t.Setenv("FUNCTION_SIGNATURE_TYPE", "test_signature")
	t.Setenv("FUNCTION_TARGET", "test_target")

	tags = service.GetTags()

	assert.Equal(t, map[string]string{
		"container_id":            "unknown",
		"location":                "unknown",
		"project_id":              "unknown",
		"origin":                  "cloudfunction",
		"service_name":            "test_service",
		"revision_name":           "test_revision",
		"configuration_name":      "test_config",
		"_dd.origin":              "cloudfunction",
		"function_target":         "test_target",
		"function_signature_type": "test_signature",
	}, tags)
}
