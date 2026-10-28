// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/agentmetadata"
	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	serverlessinitimpl "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type metadataLifecycle struct{ hooks []compdef.Hook }

func (l *metadataLifecycle) Append(hook compdef.Hook) { l.hooks = append(l.hooks, hook) }

type metadataRecordingSerializer struct {
	serializer.MetricSerializer
	payloads []*agentmetadata.Payload
}

func (s *metadataRecordingSerializer) SendMetadata(payload marshaler.JSONMarshaler) error {
	s.payloads = append(s.payloads, payload.(*agentmetadata.Payload))
	return nil
}

// gcpMetadataTransport supplies the real GCP lookups with deterministic values
// without contacting the metadata service or any other host.
type gcpMetadataTransport map[string]string

func (m gcpMetadataTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	value, ok := m[req.URL.Path]
	if req.URL.Host != "metadata.google.internal" || !ok {
		return nil, fmt.Errorf("unexpected GCP metadata request: %s", req.URL)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(value)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func clearPlatformEnv(t *testing.T) {
	for _, key := range []string{
		"AWS_LAMBDA_MICROVM_IMAGE_ARN", "K_SERVICE", "FUNCTION_TARGET", "CLOUD_RUN_JOB",
		"CONTAINER_APP_NAME", "WEBSITE_STACK", "FUNCTIONS_WORKER_RUNTIME",
	} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}

// collectPlatformMetadata warms the provider through the main tag path, runs
// one metadata collection, and stops the component. ok is false when the
// metadata payload is not eligible for collection.
func collectPlatformMetadata(t *testing.T, provider serverlessdef.CloudServiceMetadataProvider, enabled bool) (payloads []*agentmetadata.Payload, ok bool) {
	configmock.New(t)
	configureTags(provider.(cloudservice.CloudService))
	synctest.Test(t, func(t *testing.T) {
		serial := &metadataRecordingSerializer{}
		lc := &metadataLifecycle{}
		p := serverlessinitimpl.NewComponent(serverlessinitimpl.Requires{
			Lc: lc,
			Config: config.NewMockWithOverrides(t, map[string]any{
				"serverless.metadata_enabled":       enabled,
				"inventories_first_run_delay":       0,
				"inventories_configuration_enabled": false,
			}),
			Log:        logmock.New(t),
			Hostname:   fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule()),
			Params:     serverlessdef.Params{Sidecar: true, Tags: map[string]string{"service": "custom-dd-service"}},
			Provider:   provider,
			Serializer: serial,
		})
		if p.Provider.Callback == nil {
			return
		}
		ok = true
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { p.Provider.Callback(ctx); close(done) }()
		synctest.Wait()
		cancel()
		<-done
		for _, hook := range lc.hooks {
			assert.NoError(t, hook.OnStop(t.Context()))
		}
		payloads = serial.payloads
	})
	return payloads, ok
}

func TestCloudRunMetadataSerializesIdentity(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = gcpMetadataTransport{
		"/computeMetadata/v1/project/project-id": "test-project",
		"/computeMetadata/v1/instance/region":    "projects/123/regions/us-central1",
		"/computeMetadata/v1/instance/id":        "test-instance",
	}

	const gcpPrefix = "//run.googleapis.com/projects/test-project/locations/us-central1"
	for _, platform := range []struct {
		name               string
		env                map[string]string
		missingRequiredEnv string
		resourceID         string
		parentID           string
		resourceName       string
	}{
		{
			name: "cloud_run_service",
			env: map[string]string{
				"K_SERVICE": "test-service", "K_REVISION": "test-revision",
			},
			missingRequiredEnv: "K_SERVICE",
			resourceID:         gcpPrefix + "/revisions/test-revision",
			parentID:           gcpPrefix + "/services/test-service",
			resourceName:       "test-service",
		},
		{
			name: "cloud_run_function",
			env: map[string]string{
				"K_SERVICE": "test-function", "K_REVISION": "test-function-revision", "FUNCTION_TARGET": "handler",
			},
			missingRequiredEnv: "K_REVISION",
			resourceID:         gcpPrefix + "/revisions/test-function-revision",
			parentID:           gcpPrefix + "/services/test-function",
			resourceName:       "test-function",
		},
		{
			name: "cloud_run_job",
			env: map[string]string{
				"CLOUD_RUN_JOB": "test-job", "CLOUD_RUN_EXECUTION": "test-execution",
			},
			missingRequiredEnv: "CLOUD_RUN_JOB",
			resourceID:         gcpPrefix + "/executions/test-execution",
			parentID:           gcpPrefix + "/jobs/test-job",
			resourceName:       "test-job",
		},
	} {
		t.Run(platform.name, func(t *testing.T) {
			for _, missing := range []bool{false, true} {
				t.Run(fmt.Sprintf("missing_required=%t", missing), func(t *testing.T) {
					clearPlatformEnv(t)
					for key, value := range platform.env {
						t.Setenv(key, value)
					}
					service := cloudservice.GetCloudServiceType()
					// An already-identified Gen2 function needs no function target for its identity.
					require.NoError(t, os.Unsetenv("FUNCTION_TARGET"))
					if missing {
						t.Setenv(platform.missingRequiredEnv, "")
					}
					require.True(t, service.CanCollectMetadata())

					payloads, ok := collectPlatformMetadata(t, service, true)
					require.True(t, ok)
					if missing {
						assert.Empty(t, payloads, "missing identity must withhold the payload")
						return
					}
					require.Len(t, payloads, 1)
					metadata := payloads[0].Metadata
					assert.Equal(t, platform.resourceID, metadata["resource_id"])
					assert.Equal(t, platform.resourceName, metadata["resource_name"])
					assert.Equal(t, platform.name, metadata["workload_type"])
					assert.Equal(t, platform.parentID, metadata["parent_resource_id"])
					assert.Equal(t, "us-central1", metadata["region"])
					assert.Equal(t, "test-project", metadata["gcp_project_id"])
					assert.Equal(t, "custom-dd-service", metadata["dd_service"])
					assert.Equal(t, "startup", metadata["report_reason"])
				})
			}
		})
	}
}

func TestContainerAppMetadataSerializesIdentity(t *testing.T) {
	for _, scenario := range []string{"valid", "missing revision", "metadata disabled"} {
		t.Run(scenario, func(t *testing.T) {
			clearPlatformEnv(t)
			t.Setenv("DD_AZURE_SUBSCRIPTION_ID", "Test-Subscription")
			t.Setenv("DD_AZURE_RESOURCE_GROUP", "Test-Group")
			t.Setenv("CONTAINER_APP_NAME", "Test-App")
			t.Setenv("CONTAINER_APP_REVISION", "Test-Revision")
			t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "environment.eastus.azurecontainerapps.io")
			service := cloudservice.GetCloudServiceType()
			require.IsType(t, &cloudservice.ContainerApp{}, service)
			if scenario == "missing revision" {
				t.Setenv("CONTAINER_APP_REVISION", "")
			}

			payloads, ok := collectPlatformMetadata(t, service, scenario != "metadata disabled")
			if scenario == "metadata disabled" {
				assert.False(t, ok, "disabled metadata must not register periodic collection")
				return
			}
			require.True(t, ok)
			if scenario == "missing revision" {
				assert.Empty(t, payloads, "missing identity must withhold the payload")
				return
			}
			require.Len(t, payloads, 1)
			metadata := payloads[0].Metadata
			const parent = "/subscriptions/test-subscription/resourcegroups/test-group/providers/microsoft.app/containerapps/test-app"
			assert.Equal(t, parent+"/revisions/Test-Revision", metadata["resource_id"])
			assert.Equal(t, parent, metadata["parent_resource_id"])
			assert.Equal(t, "Test-App", metadata["resource_name"])
			assert.Equal(t, "azure_container_app", metadata["workload_type"])
			assert.Equal(t, "eastus", metadata["region"])
			assert.Equal(t, "Test-Subscription", metadata["azure_subscription_id"])
			assert.Equal(t, "Test-Group", metadata["azure_resource_group"])
			assert.Equal(t, "custom-dd-service", metadata["dd_service"])
			assert.Equal(t, "startup", metadata["report_reason"])
		})
	}
}

func TestAppServiceMetadataSerializesIdentity(t *testing.T) {
	for _, platform := range []struct {
		name       string
		worker     string
		resourceID string
	}{
		{
			name:       "azure_app_service",
			resourceID: "/subscriptions/test-subscription/resourcegroups/test-group/providers/microsoft.web/sites/test-site",
		},
		{
			name:       "azure_function",
			worker:     "node",
			resourceID: "/subscriptions/Test-Subscription/resourcegroups/Test-Group/providers/microsoft.web/sites/Test-Site",
		},
	} {
		t.Run(platform.name, func(t *testing.T) {
			for _, scenario := range []string{"valid", "missing site", "metadata disabled"} {
				t.Run(scenario, func(t *testing.T) {
					clearPlatformEnv(t)
					t.Setenv("WEBSITE_OWNER_NAME", "Test-Subscription+webspace")
					t.Setenv("WEBSITE_RESOURCE_GROUP", "Test-Group")
					t.Setenv("WEBSITE_SITE_NAME", "Test-Site")
					t.Setenv("WEBSITE_STACK", "NODE")
					if platform.worker != "" {
						t.Setenv("FUNCTIONS_WORKER_RUNTIME", platform.worker)
					}
					service := cloudservice.GetCloudServiceType()
					require.IsType(t, &cloudservice.AppService{}, service)
					if scenario == "missing site" {
						t.Setenv("WEBSITE_SITE_NAME", "")
					}

					payloads, ok := collectPlatformMetadata(t, service, scenario != "metadata disabled")
					if scenario == "metadata disabled" {
						assert.False(t, ok, "disabled metadata must not register periodic collection")
						return
					}
					require.True(t, ok)
					if scenario == "missing site" {
						assert.Empty(t, payloads, "missing identity must withhold the payload")
						return
					}
					require.Len(t, payloads, 1)
					metadata := payloads[0].Metadata
					assert.Equal(t, platform.resourceID, metadata["resource_id"])
					assert.Nil(t, metadata["parent_resource_id"])
					assert.Equal(t, "Test-Site", metadata["resource_name"])
					assert.Equal(t, platform.name, metadata["workload_type"])
					assert.Equal(t, "custom-dd-service", metadata["dd_service"])
					assert.Equal(t, "startup", metadata["report_reason"])
				})
			}
		})
	}
}
