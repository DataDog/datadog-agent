// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"encoding/json"
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
	payloads []*sentMetadataPayload
}

func (s *metadataRecordingSerializer) SendMetadata(payload marshaler.JSONMarshaler) error {
	sent, err := decodeMetadataPayload(payload)
	if err != nil {
		return err
	}
	s.payloads = append(s.payloads, sent)
	return nil
}

// sentMetadataPayload holds the fields of a sent metadata payload that the
// tests check, decoded from its JSON.
type sentMetadataPayload struct {
	UUID     string                 `json:"uuid"`
	Metadata map[string]interface{} `json:"agent_metadata"`
}

func decodeMetadataPayload(payload marshaler.JSONMarshaler) (*sentMetadataPayload, error) {
	raw, err := payload.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var sent sentMetadataPayload
	if err := json.Unmarshal(raw, &sent); err != nil {
		return nil, err
	}
	return &sent, nil
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
// one metadata collection, and stops the component.
func collectPlatformMetadata(t *testing.T, provider serverlessdef.CloudServiceMetadataProvider) (payloads []*sentMetadataPayload) {
	configmock.New(t)
	configureTags(provider.(cloudservice.CloudService))
	synctest.Test(t, func(t *testing.T) {
		serial := &metadataRecordingSerializer{}
		lc := &metadataLifecycle{}
		p := serverlessinitimpl.NewComponent(serverlessinitimpl.Requires{
			Lc: lc,
			Config: config.NewMockWithOverrides(t, map[string]any{
				"serverless.metadata_enabled":       true,
				"inventories_first_run_delay":       0,
				"inventories_configuration_enabled": false,
			}),
			Log:        logmock.New(t),
			Hostname:   fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule()),
			Params:     serverlessdef.Params{Sidecar: true, Tags: map[string]string{"service": "custom-dd-service"}},
			Provider:   provider,
			Serializer: serial,
		})
		require.NotNil(t, p.Provider.Callback)
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
	return payloads
}

func TestPlatformMetadataSerializesIdentity(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = gcpMetadataTransport{
		"/computeMetadata/v1/project/project-id": "test-project",
		"/computeMetadata/v1/instance/region":    "projects/123/regions/us-central1",
		"/computeMetadata/v1/instance/id":        "test-instance",
	}

	const gcpPrefix = "//run.googleapis.com/projects/test-project/locations/us-central1"
	const containerApp = "/subscriptions/test-subscription/resourcegroups/test-group/providers/microsoft.app/containerapps/test-app"
	appServiceEnv := map[string]string{
		"WEBSITE_OWNER_NAME": "Test-Subscription+webspace", "WEBSITE_RESOURCE_GROUP": "Test-Group",
		"WEBSITE_SITE_NAME": "Test-Site", "WEBSITE_STACK": "NODE",
	}
	for _, platform := range []struct {
		workloadType string
		env          map[string]string
		resourceID   string
		parentID     any
	}{
		{
			workloadType: "cloud_run_service",
			env:          map[string]string{"K_SERVICE": "test-service", "K_REVISION": "test-revision"},
			resourceID:   gcpPrefix + "/revisions/test-revision",
			parentID:     gcpPrefix + "/services/test-service",
		},
		{
			workloadType: "cloud_run_function",
			env:          map[string]string{"K_SERVICE": "test-function", "K_REVISION": "test-function-revision", "FUNCTION_TARGET": "handler"},
			resourceID:   gcpPrefix + "/revisions/test-function-revision",
			parentID:     gcpPrefix + "/services/test-function",
		},
		{
			workloadType: "cloud_run_job",
			env:          map[string]string{"CLOUD_RUN_JOB": "test-job", "CLOUD_RUN_EXECUTION": "test-execution"},
			resourceID:   gcpPrefix + "/executions/test-execution",
			parentID:     gcpPrefix + "/jobs/test-job",
		},
		{
			workloadType: "azure_container_app",
			env: map[string]string{
				"DD_AZURE_SUBSCRIPTION_ID": "Test-Subscription", "DD_AZURE_RESOURCE_GROUP": "Test-Group",
				"CONTAINER_APP_NAME": "Test-App", "CONTAINER_APP_REVISION": "Test-Revision",
				"CONTAINER_APP_ENV_DNS_SUFFIX": "environment.eastus.azurecontainerapps.io",
			},
			resourceID: containerApp + "/revisions/Test-Revision",
			parentID:   containerApp,
		},
		{
			workloadType: "azure_app_service",
			env:          appServiceEnv,
			resourceID:   "/subscriptions/test-subscription/resourcegroups/test-group/providers/microsoft.web/sites/test-site",
		},
		{
			workloadType: "azure_function",
			env:          map[string]string{"FUNCTIONS_WORKER_RUNTIME": "node"},
			resourceID:   "/subscriptions/Test-Subscription/resourcegroups/Test-Group/providers/microsoft.web/sites/Test-Site",
		},
	} {
		t.Run(platform.workloadType, func(t *testing.T) {
			clearPlatformEnv(t)
			if platform.workloadType == "azure_function" {
				for key, value := range appServiceEnv {
					t.Setenv(key, value)
				}
			}
			for key, value := range platform.env {
				t.Setenv(key, value)
			}
			service := cloudservice.GetCloudServiceType()
			// An already-identified Gen2 function needs no function target for its identity.
			require.NoError(t, os.Unsetenv("FUNCTION_TARGET"))
			require.True(t, service.CanCollectMetadata())

			payloads := collectPlatformMetadata(t, service)
			require.Len(t, payloads, 1)
			metadata := payloads[0].Metadata
			assert.Equal(t, platform.workloadType, metadata["workload_type"])
			assert.Equal(t, platform.resourceID, metadata["resource_id"])
			assert.Equal(t, platform.parentID, metadata["parent_resource_id"])
			assert.Equal(t, "custom-dd-service", metadata["dd_service"])
		})
	}
}
