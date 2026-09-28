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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	serverlessInitInventory "github.com/DataDog/datadog-agent/cmd/serverless-init/inventory"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	inventoryagentimpl "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/impl"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// inventoryMetadataTransport supplies the real GCP builders with deterministic
// metadata without contacting the cloud metadata service or any other host.
type inventoryMetadataTransport map[string]string

func (m inventoryMetadataTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	value, ok := m[req.URL.Path]
	if req.URL.Host != "metadata.google.internal" || !ok {
		return nil, fmt.Errorf("unexpected inventory metadata request: %s", req.URL)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(value)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestInventorySerializesCloudRunIdentity(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = inventoryMetadataTransport{
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
					for _, key := range []string{
						"AWS_LAMBDA_MICROVM_IMAGE_ARN", "K_SERVICE", "FUNCTION_TARGET", "CLOUD_RUN_JOB",
						"CONTAINER_APP_NAME", "WEBSITE_STACK", "FUNCTIONS_WORKER_RUNTIME",
					} {
						t.Setenv(key, "")
						require.NoError(t, os.Unsetenv(key))
					}
					for key, value := range platform.env {
						t.Setenv(key, value)
					}
					service := cloudservice.GetCloudServiceType()
					// An already-identified Gen2 function needs no function target for inventory.
					require.NoError(t, os.Unsetenv("FUNCTION_TARGET"))
					if missing {
						t.Setenv(platform.missingRequiredEnv, "")
					}
					require.Equal(t, !missing, service.CanCollectInventory())

					conf := coreconfig.NewMock(t)
					for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
						conf.Set(key, true, configmodel.SourceAgentRuntime)
					}
					conf.Set("inventories_first_run_delay", 0, configmodel.SourceAgentRuntime)
					conf.Set("inventories_configuration_enabled", false, configmodel.SourceAgentRuntime)
					configureInventory(service)
					serial := &inventoryRecordingSerializer{}
					hostname, _ := hostnamemock.NewMock("inventory-test")
					provides := inventoryagentimpl.NewComponent(inventoryagentimpl.Requires{
						Config: conf, Log: logmock.New(t), Hostname: hostname, Serializer: serial, Capabilities: serverlessInitInventory.NewCapabilities(),
					})
					serverlessInitInventory.Inject(provides.Comp, service, mode.Conf{SidecarMode: true}, conf, map[string]string{"service": "custom-dd-service"})
					serverlessInitInventory.Submit(provides.Comp, conf)
					if missing {
						assert.Empty(t, serial.payloads)
						assert.Nil(t, provides.Provider.Callback, "missing identity must suppress the periodic provider too")
						return
					}
					require.Len(t, serial.payloads, 1)
					require.NotNil(t, provides.Provider.Callback)
					provides.Provider.Callback(context.Background())
					require.Len(t, serial.payloads, 2)
					for _, data := range serial.payloads {
						var payload struct {
							Metadata map[string]interface{} `json:"agent_metadata"`
						}
						require.NoError(t, json.Unmarshal(data, &payload))
						assert.Equal(t, platform.resourceID, payload.Metadata["resource_id"])
						assert.Equal(t, platform.resourceName, payload.Metadata["resource_name"])
						assert.Equal(t, platform.name, payload.Metadata["workload_type"])
						assert.Equal(t, platform.parentID, payload.Metadata["parent_resource_id"])
						assert.Equal(t, "custom-dd-service", payload.Metadata["dd_service"])
						assert.NotContains(t, payload.Metadata, "deployment_id")
					}
				})
			}
		})
	}
}
