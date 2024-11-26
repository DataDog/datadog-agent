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
	service := &CloudRun{spanNamespace: cloudRunService}

	metadataHelperFunc = func(*helper.GCPConfig) *helper.GCPMetadata {
		return &helper.GCPMetadata{
			ContainerID: &helper.Info{
				TagName: "container_id",
				Value:   "test_container",
			},
			Region: &helper.Info{
				TagName: "location",
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
		"container_id":          "test_container",
		"gcr.container_id":      "test_container",
		"gcr.location":          "test_region",
		"location":              "test_region",
		"project_id":            "test_project",
		"gcr.project_id":        "test_project",
		"origin":                "cloudrun",
		"_dd.origin":            "cloudrun",
		"_dd.gcr.resource_name": "projects/test_project/locations/test_region/services/",
	}, tags)
}

func TestGetCloudRunTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRun{spanNamespace: cloudRunService}

	metadataHelperFunc = func(*helper.GCPConfig) *helper.GCPMetadata {
		return &helper.GCPMetadata{
			ContainerID: &helper.Info{
				TagName: "container_id",
				Value:   "test_container",
			},
			Region: &helper.Info{
				TagName: "location",
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
		"container_id":          "test_container",
		"gcr.container_id":      "test_container",
		"location":              "test_region",
		"gcr.location":          "test_region",
		"project_id":            "test_project",
		"gcr.project_id":        "test_project",
		"service_name":          "test_service",
		"gcr.service_name":      "test_service",
		"gcr.revision_name":     "test_revision",
		"revision_name":         "test_revision",
		"origin":                "cloudrun",
		"_dd.origin":            "cloudrun",
		"_dd.gcr.resource_name": "projects/test_project/locations/test_region/services/test_service",
	}, tags)
}

func TestGetCloudRunFunctionTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRun{spanNamespace: cloudRunFunction}

	metadataHelperFunc = func(*helper.GCPConfig) *helper.GCPMetadata {
		return &helper.GCPMetadata{
			ContainerID: &helper.Info{
				TagName: "container_id",
				Value:   "test_container",
			},
			Region: &helper.Info{
				TagName: "location",
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
		"container_id":                  "test_container",
		"gcrfx.container_id":            "test_container",
		"location":                      "test_region",
		"gcrfx.location":                "test_region",
		"origin":                        "cloudrun",
		"project_id":                    "test_project",
		"gcrfx.project_id":              "test_project",
		"service_name":                  "test_service",
		"gcrfx.service_name":            "test_service",
		"revision_name":                 "test_revision",
		"gcrfx.revision_name":           "test_revision",
		"configuration_name":            "test_config",
		"gcrfx.configuration_name":      "test_config",
		"_dd.origin":                    "cloudrun",
		"gcrfx.function_target":         "test_target",
		"gcrfx.function_signature_type": "test_signature",
		"_dd.gcrfx.resource_name":       "projects/test_project/locations/test_region/services/test_service/functions/test_target",
	}, tags)
}
