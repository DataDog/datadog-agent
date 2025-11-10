// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	assert.Equal(t, 300*time.Millisecond, GetDefaultConfig().timeout)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/instance/id", GetDefaultConfig().containerIDURL)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/project/project-id", GetDefaultConfig().projectIDURL)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/instance/region", GetDefaultConfig().regionURL)
}

func TestGetSingleMetadataMalformedUrl(t *testing.T) {
	assert.Equal(t, "unknown", getSingleMetadata(&http.Client{}, string([]byte("\u007F"))))
}

func TestSingleMetadataTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer ts.Close()
	httpClient := &http.Client{
		Timeout: 1 * time.Nanosecond,
	}
	assert.Equal(t, "unknown", getSingleMetadata(httpClient, ts.URL))
}

func TestSingleMetadataOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("1234"))
	}))
	defer ts.Close()
	assert.Equal(t, "1234", getSingleMetadata(&http.Client{}, ts.URL))
}

func TestGetRegionUnknown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("unknown"))
	}))
	defer ts.Close()
	testConfig := &GCPConfig{
		timeout:   1 * time.Second,
		regionURL: ts.URL,
	}
	assert.Equal(t, "unknown", getRegion(&http.Client{}, testConfig.regionURL))
}

func TestGetRegionOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("projects/xxx/regions/us-central1"))
	}))
	defer ts.Close()
	testConfig := &GCPConfig{
		timeout:   1 * time.Second,
		regionURL: ts.URL,
	}
	assert.Equal(t, "us-central1", getRegion(&http.Client{}, testConfig.regionURL))
}

func TestGetMetaDataComplete(t *testing.T) {
	tsProjectID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("superProjectID"))
	}))
	defer tsProjectID.Close()
	tsRegion := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("greatRegion"))
	}))
	defer tsRegion.Close()
	tsContainerID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("acb54"))
	}))
	defer tsContainerID.Close()

	testConfig := &GCPConfig{
		timeout:        1 * time.Second,
		projectIDURL:   tsProjectID.URL,
		regionURL:      tsRegion.URL,
		containerIDURL: tsContainerID.URL,
	}

	expected := map[string]string{
		"container_id":     "acb54",
		"location":         "greatregion",
		"project_id":       "superprojectid",
		"gcr.container_id": "acb54",
		"gcr.location":     "greatregion",
		"gcr.project_id":   "superprojectid",
	}

	metadata := GetMetaData(testConfig, true)
	assert.Equal(t, expected, metadata)
}

func TestGetMetaDataIncompleteDueToTimeout(t *testing.T) {
	tsProjectID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("superProjectID"))
	}))
	defer tsProjectID.Close()
	tsRegion := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1 * time.Second)
		w.Write([]byte("greatRegion"))
	}))
	defer tsRegion.Close()
	tsContainerID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("acb54"))
	}))
	defer tsContainerID.Close()

	testConfig := &GCPConfig{
		timeout:        500 * time.Millisecond,
		projectIDURL:   tsProjectID.URL,
		regionURL:      tsRegion.URL,
		containerIDURL: tsContainerID.URL,
	}
	expected := map[string]string{
		"gcr.container_id": "acb54",
		"gcr.location":     "unknown",
		"gcr.project_id":   "superprojectid",
		"container_id":     "acb54",
		"location":         "unknown",
		"project_id":       "superprojectid",
	}

	metadata := GetMetaData(testConfig, true)
	assert.Equal(t, expected, metadata)
}

func TestGetCloudRunTags(t *testing.T) {
	service := &CloudRun{spanNamespace: cloudRunService}

	metadataHelperFunc = func(*GCPConfig, bool) map[string]string {
		return map[string]string{
			"container_id":     "test_container",
			"location":         "test_region",
			"project_id":       "test_project",
			"gcr.container_id": "test_container",
			"gcr.location":     "test_region",
			"gcr.project_id":   "test_project",
		}
	}

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"container_id":      "test_container",
		"gcr.container_id":  "test_container",
		"gcr.location":      "test_region",
		"location":          "test_region",
		"project_id":        "test_project",
		"gcr.project_id":    "test_project",
		"origin":            "cloudrun",
		"_dd.origin":        "cloudrun",
		"gcr.resource_name": "projects/test_project/locations/test_region/services/",
	}, tags)
}

func TestGetCloudRunTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRun{spanNamespace: cloudRunService}

	metadataHelperFunc = func(*GCPConfig, bool) map[string]string {
		return map[string]string{
			"container_id":     "test_container",
			"location":         "test_region",
			"project_id":       "test_project",
			"gcr.container_id": "test_container",
			"gcr.location":     "test_region",
			"gcr.project_id":   "test_project",
		}
	}

	t.Setenv("K_SERVICE", "test_service")
	t.Setenv("K_REVISION", "test_revision")

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"container_id":      "test_container",
		"gcr.container_id":  "test_container",
		"location":          "test_region",
		"gcr.location":      "test_region",
		"project_id":        "test_project",
		"gcr.project_id":    "test_project",
		"service_name":      "test_service",
		"gcr.service_name":  "test_service",
		"gcr.revision_name": "test_revision",
		"revision_name":     "test_revision",
		"origin":            "cloudrun",
		"_dd.origin":        "cloudrun",
		"gcr.resource_name": "projects/test_project/locations/test_region/services/test_service",
	}, tags)
}

func TestGetCloudRunFunctionTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRun{spanNamespace: cloudRunFunction}

	metadataHelperFunc = func(*GCPConfig, bool) map[string]string {
		return map[string]string{
			"container_id":       "test_container",
			"location":           "test_region",
			"project_id":         "test_project",
			"gcrfx.container_id": "test_container",
			"gcrfx.location":     "test_region",
			"gcrfx.project_id":   "test_project",
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
		"_dd.origin":                    "cloudrun",
		"project_id":                    "test_project",
		"gcrfx.project_id":              "test_project",
		"service_name":                  "test_service",
		"gcrfx.service_name":            "test_service",
		"revision_name":                 "test_revision",
		"gcrfx.revision_name":           "test_revision",
		"configuration_name":            "test_config",
		"gcrfx.configuration_name":      "test_config",
		"gcrfx.build_function_target":   "test_target",
		"gcrfx.function_signature_type": "test_signature",
		"gcrfx.resource_name":           "projects/test_project/locations/test_region/services/test_service/functions/test_target",
	}, tags)
}
