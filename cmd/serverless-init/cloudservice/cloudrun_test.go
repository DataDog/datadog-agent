// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
)

func TestDefaultConfig(t *testing.T) {
	assert.Equal(t, 300*time.Millisecond, GetDefaultConfig().timeout)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/instance/id", GetDefaultConfig().containerIDURL)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/project/project-id", GetDefaultConfig().projectIDURL)
	assert.Equal(t, "http://metadata.google.internal/computeMetadata/v1/instance/region", GetDefaultConfig().regionURL)
}

func TestGetSingleMetadataMalformedUrl(t *testing.T) {
	assert.Empty(t, getSingleMetadata(&http.Client{}, string([]byte("\u007F"))))
}

func TestSingleMetadataTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer ts.Close()
	httpClient := &http.Client{
		Timeout: 1 * time.Nanosecond,
	}
	assert.Empty(t, getSingleMetadata(httpClient, ts.URL))
}

func TestSingleMetadataOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("1234"))
	}))
	defer ts.Close()
	assert.Equal(t, "1234", getSingleMetadata(&http.Client{}, ts.URL))
}

func TestSingleMetadataHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	assert.Empty(t, getSingleMetadata(ts.Client(), ts.URL))
	assert.Empty(t, getRegion(ts.Client(), ts.URL))
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

	metadata := GetMetaData(testConfig, CloudRunService)
	assert.Equal(t, expected, metadata)
}

func TestGetMetaDataIncompleteDueToTimeout(t *testing.T) {
	tsProjectID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("superProjectID"))
	}))
	defer tsProjectID.Close()
	tsRegion := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
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
		"gcr.location":     "",
		"gcr.project_id":   "superprojectid",
		"container_id":     "acb54",
		"location":         "",
		"project_id":       "superprojectid",
	}

	metadata := GetMetaData(testConfig, CloudRunService)
	assert.Equal(t, expected, metadata)
}

func TestGetMetaDataCloudRunTypePrefixes(t *testing.T) {
	tsProjectID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("testprojectid"))
	}))
	defer tsProjectID.Close()
	tsRegion := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("testregion"))
	}))
	defer tsRegion.Close()
	tsContainerID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("testcontainerid"))
	}))
	defer tsContainerID.Close()

	testConfig := &GCPConfig{
		timeout:        1 * time.Second,
		projectIDURL:   tsProjectID.URL,
		regionURL:      tsRegion.URL,
		containerIDURL: tsContainerID.URL,
	}

	for _, tc := range []struct {
		cloudRunType CloudRunType
		tagPrefix    string
	}{
		{CloudRunService, cloudRunServiceTagPrefix},
		{CloudRunFunction, cloudRunFunctionTagPrefix},
		{CloudRunJob, cloudRunJobTagPrefix},
	} {
		meta := GetMetaData(testConfig, tc.cloudRunType)
		assert.Equal(t, "testcontainerid", meta[tc.tagPrefix+containerID], "cloudRunType=%v", tc.cloudRunType)
		assert.Equal(t, "testregion", meta[tc.tagPrefix+location], "cloudRunType=%v", tc.cloudRunType)
		assert.Equal(t, "testprojectid", meta[tc.tagPrefix+projectID], "cloudRunType=%v", tc.cloudRunType)
	}
}

func TestGetCloudRunTags(t *testing.T) {
	service := &CloudRun{}

	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
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
		"container_id":     "test_container",
		"gcr.container_id": "test_container",
		"gcr.location":     "test_region",
		"location":         "test_region",
		"project_id":       "test_project",
		"gcr.project_id":   "test_project",
		"origin":           "cloudrun",
		"_dd.origin":       "cloudrun",
	}, tags)
}

func TestGetCloudRunTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRun{}

	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
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
	service := &CloudRun{isFunction: true}

	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
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

func TestCloudRunServiceGetMetadata(t *testing.T) {
	service := &CloudRun{}

	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
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

	service.GetTags()
	metadata := service.GetMetadata()

	assert.Equal(t, serverlessdef.CloudServiceMetadata{
		WorkloadType:     workloadTypeCloudRunService,
		ResourceID:       "//run.googleapis.com/projects/test_project/locations/test_region/revisions/test_revision",
		ParentResourceID: "//run.googleapis.com/projects/test_project/locations/test_region/services/test_service",
		ResourceName:     "test_service",
		Region:           "test_region",
		GCPProjectID:     "test_project",
	}, metadata)
	assert.True(t, service.CanCollectMetadata())
}

func TestCloudRunFunctionGetMetadata(t *testing.T) {
	service := &CloudRun{isFunction: true}

	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
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
	t.Setenv("FUNCTION_TARGET", "test_target")

	service.GetTags()
	metadata := service.GetMetadata()

	assert.Equal(t, serverlessdef.CloudServiceMetadata{
		WorkloadType:     workloadTypeCloudRunFunction,
		ResourceID:       "//run.googleapis.com/projects/test_project/locations/test_region/revisions/test_revision",
		ParentResourceID: "//run.googleapis.com/projects/test_project/locations/test_region/services/test_service",
		ResourceName:     "test_service",
		Region:           "test_region",
		GCPProjectID:     "test_project",
	}, metadata)
	assert.True(t, service.CanCollectMetadata())
}

func TestCloudRunFunctionMetadataIndependentOfFunctionTarget(t *testing.T) {
	saved := metadataHelperFunc
	t.Cleanup(func() { metadataHelperFunc = saved })
	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
		return map[string]string{projectID: "project", location: "region"}
	}
	t.Setenv(ServiceNameEnvVar, "service")
	t.Setenv(revisionNameEnvVar, "revision")
	t.Setenv(functionTargetEnvVar, "target")
	service := &CloudRun{isFunction: true}
	service.GetTags()
	first := service.GetMetadata()
	require.Equal(t, "//run.googleapis.com/projects/project/locations/region/revisions/revision", first.ResourceID)

	for _, target := range []struct {
		name  string
		value string
		unset bool
	}{
		{name: "changed", value: "another_target"},
		{name: "empty"},
		{name: "unset", unset: true},
	} {
		t.Run(target.name, func(t *testing.T) {
			t.Setenv(functionTargetEnvVar, target.value)
			if target.unset {
				require.NoError(t, os.Unsetenv(functionTargetEnvVar))
			}
			assert.Equal(t, first, service.GetMetadata())
		})
	}
}

func TestCloudRunMissingIdentity(t *testing.T) {
	saved := metadataHelperFunc
	t.Cleanup(func() { metadataHelperFunc = saved })
	for _, kind := range []CloudRunType{CloudRunService, CloudRunFunction, CloudRunJob} {
		t.Run(string(kind), func(t *testing.T) {
			env := map[string]string{
				ServiceNameEnvVar: "unknown", revisionNameEnvVar: "revision", functionTargetEnvVar: "target",
				cloudRunJobNameEnvVar: "job", cloudRunExecutionEnvVar: "execution",
			}
			for key, value := range env {
				t.Setenv(key, value)
			}
			keys := []string{projectID, location, ServiceNameEnvVar, revisionNameEnvVar}
			if kind == CloudRunJob {
				keys = []string{projectID, location, cloudRunJobNameEnvVar, cloudRunExecutionEnvVar}
			}
			for _, missing := range append([]string{"none", containerID, functionTargetEnvVar}, keys...) {
				t.Run(missing, func(t *testing.T) {
					values := map[string]string{projectID: "unknown", location: "region", containerID: "instance"}
					delete(values, missing)
					if _, ok := env[missing]; ok {
						t.Setenv(missing, "")
					}
					metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string { return values }
					var service CloudService = &CloudRun{isFunction: kind == CloudRunFunction}
					if kind == CloudRunJob {
						service = &CloudRunJobs{}
					}
					service.GetTags()
					id := service.GetMetadata().ResourceID
					if missing == "none" || missing == containerID || missing == functionTargetEnvVar {
						assert.NotEmpty(t, id)
					} else {
						assert.Empty(t, id)
					}
					assert.True(t, service.CanCollectMetadata(), "identity readiness does not change platform support")
				})
			}
		})
	}
}

func TestCloudRunTagsOmitFailedMetadataLookups(t *testing.T) {
	saved := metadataHelperFunc
	t.Cleanup(func() { metadataHelperFunc = saved })
	t.Setenv(ServiceNameEnvVar, "service")
	t.Setenv(revisionNameEnvVar, "revision")
	t.Setenv(cloudRunJobNameEnvVar, "job")
	for _, tt := range []struct {
		kind    CloudRunType
		prefix  string
		service CloudService
	}{
		{CloudRunService, cloudRunServiceTagPrefix, &CloudRun{}},
		{CloudRunFunction, cloudRunFunctionTagPrefix, &CloudRun{isFunction: true}},
		{CloudRunJob, cloudRunJobTagPrefix, &CloudRunJobs{}},
	} {
		t.Run(string(tt.kind), func(t *testing.T) {
			metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
				return map[string]string{
					projectID: "", tt.prefix + projectID: "",
					location: "region", tt.prefix + location: "region",
					containerID: "", tt.prefix + containerID: "",
				}
			}
			tags := tt.service.GetTags()
			for _, key := range []string{projectID, containerID} {
				assert.NotContains(t, tags, key)
				assert.NotContains(t, tags, tt.prefix+key)
			}
			assert.NotContains(t, tags, tt.prefix+resourceName)
			assert.Equal(t, "region", tags[location])
			assert.Equal(t, "region", tags[tt.prefix+location])
			assert.Empty(t, tt.service.GetMetadata().ResourceID)
		})
	}
}

func TestCloudRunMetadataSharesTagLookup(t *testing.T) {
	for _, isFunction := range []bool{false, true} {
		for _, available := range []bool{false, true} {
			t.Run(fmt.Sprintf("function=%t/metadata=%t", isFunction, available), func(t *testing.T) {
				t.Setenv(ServiceNameEnvVar, "service")
				t.Setenv(revisionNameEnvVar, "revision")
				t.Setenv(functionTargetEnvVar, "handler")
				service := &CloudRun{isFunction: isFunction}
				prefix := cloudRunServiceTagPrefix
				kind := CloudRunService
				if isFunction {
					prefix = cloudRunFunctionTagPrefix
					kind = CloudRunFunction
				}
				gcpMetadata := map[string]string{projectID: "", location: "", containerID: ""}
				if available {
					gcpMetadata = map[string]string{projectID: "project", location: "region", containerID: "instance"}
				}
				saved := metadataHelperFunc
				t.Cleanup(func() { metadataHelperFunc = saved })
				calls := 0
				metadataHelperFunc = func(_ *GCPConfig, gotKind CloudRunType) map[string]string {
					calls++
					assert.Equal(t, kind, gotKind)
					return gcpMetadata
				}

				assert.Empty(t, service.GetMetadata().ResourceID, "metadata before GetTags has no GCP values")
				assert.Zero(t, calls, "GetMetadata must not fetch GCP metadata")

				first := service.GetTags()
				first[projectID] = "mutated-by-caller"
				second := service.GetTags()
				assert.Equal(t, gcpMetadata[projectID], second[projectID])
				assert.NotContains(t, gcpMetadata, "origin", "tag enrichment must not mutate cached metadata")
				assert.Equal(t, CloudRunOrigin, second["origin"])
				assert.Equal(t, "service", second[prefix+serviceName])
				assert.Equal(t, "revision", second[prefix+revisionName])
				if available {
					resource := "projects/project/locations/region/services/service"
					if isFunction {
						resource += "/functions/handler"
					}
					assert.Equal(t, resource, second[prefix+resourceName])
					assert.NotEmpty(t, service.GetMetadata().ResourceID)
				} else {
					assert.NotContains(t, second, prefix+resourceName)
					assert.Equal(t, "unknown", service.GetEnhancedMetricTags(second).Base[projectID])
					assert.Empty(t, service.GetMetadata().ResourceID)
				}
				assert.Equal(t, 1, calls, "metadata and telemetry share the cached lookup, including failures")
			})
		}
	}
}

func TestCloudRunShutdownEmitsMetrics(t *testing.T) {
	skipOnWindows(t)
	demux := createDemultiplexer(t)
	agent := &serverlessMetrics.ServerlessMetricAgent{Demux: demux}

	service := &CloudRun{}
	service.Shutdown(agent, true, nil)

	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Empty(t, timedMetrics)
	assert.Len(t, generatedMetrics, 2)

	foundShutdown := false
	for _, sample := range generatedMetrics {
		if sample.Name == cloudRunShutdownMetricName {
			foundShutdown = true
		}
	}
	assert.True(t, foundShutdown, "shutdown metric not emitted")
}

func TestCloudRunShutdownNilMetricAgent(t *testing.T) {
	service := &CloudRun{}
	require.NotPanics(t, func() {
		service.Shutdown(nil, true, nil)
	})
}
