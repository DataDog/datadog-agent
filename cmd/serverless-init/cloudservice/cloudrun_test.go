// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice/helper"
	"github.com/stretchr/testify/assert"
)

func TestGetCloudRunTags(t *testing.T) {
	service := &CloudRun{}

	metadataHelperFunc = func(*helper.GCPConfig) *helper.GCPMetadata {
		return &helper.GCPMetadata{
			ContainerID: &helper.Info{
				TagName: "container_id",
				Value:   "test_container",
			},
			Region: &helper.Info{
				TagName: "region",
				Value:   "test_region",
			},
			ProjectID: &helper.Info{
				TagName: "project_id",
				Value:   "test_project",
			},
		}
	}

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"container_id": "test_container",
		"region":       "test_region",
		"origin":       "cloudrun",
		"project_id":   "test_project",
		"_dd.origin":   "cloudrun",
	}, tags)
}

func TestGetCloudRunTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRun{}

	metadataHelperFunc = func(*helper.GCPConfig) *helper.GCPMetadata {
		return &helper.GCPMetadata{
			ContainerID: &helper.Info{
				TagName: "container_id",
				Value:   "test_container",
			},
			Region: &helper.Info{
				TagName: "region",
				Value:   "test_region",
			},
			ProjectID: &helper.Info{
				TagName: "project_id",
				Value:   "test_project",
			},
		}
	}

	t.Setenv("K_SERVICE", "test_service")
	t.Setenv("K_REVISION", "test_revision")

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"container_id":  "test_container",
		"region":        "test_region",
		"origin":        "cloudrun",
		"project_id":    "test_project",
		"service_name":  "test_service",
		"revision_name": "test_revision",
		"_dd.origin":    "cloudrun",
	}, tags)
}

func TestGetCloudRunFunctionTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRun{cloudRunFunctionMode: true}

	metadataHelperFunc = func(*helper.GCPConfig) *helper.GCPMetadata {
		return &helper.GCPMetadata{
			ContainerID: &helper.Info{
				TagName: "container_id",
				Value:   "test_container",
			},
			Region: &helper.Info{
				TagName: "region",
				Value:   "test_region",
			},
			ProjectID: &helper.Info{
				TagName: "project_id",
				Value:   "test_project",
			},
		}
	}

	t.Setenv("K_SERVICE", "test_service")
	t.Setenv("K_REVISION", "test_revision")
	t.Setenv("K_CONFIGURATION", "test_config")
	t.Setenv("FUNCTION_SIGNATURE_TYPE", "test_signature")
	t.Setenv("FUNCTION_TARGET", "test_target")

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"container_id":            "test_container",
		"region":                  "test_region",
		"origin":                  "cloudfunctions",
		"project_id":              "test_project",
		"service_name":            "test_service",
		"revision_name":           "test_revision",
		"configuration_name":      "test_config",
		"_dd.origin":              "cloudfunctions",
		"function_target":         "test_target",
		"function_signature_type": "test_signature",
	}, tags)
}
