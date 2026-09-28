// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"os"
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

func TestInventorySerializesAppServiceIdentity(t *testing.T) {
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
			for _, scenario := range []string{"valid", "missing site", "inventory disabled"} {
				t.Run(scenario, func(t *testing.T) {
					for _, key := range []string{
						"AWS_LAMBDA_MICROVM_IMAGE_ARN", "K_SERVICE", "FUNCTION_TARGET", "CLOUD_RUN_JOB",
						"CONTAINER_APP_NAME", "WEBSITE_STACK", "FUNCTIONS_WORKER_RUNTIME",
					} {
						t.Setenv(key, "")
						require.NoError(t, os.Unsetenv(key))
					}
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
					require.Equal(t, scenario != "missing site", service.CanCollectInventory())

					conf := coreconfig.NewMock(t)
					for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
						conf.Set(key, true, configmodel.SourceAgentRuntime)
					}
					if scenario == "inventory disabled" {
						conf.Set("serverless.inventory_enabled", false, configmodel.SourceAgentRuntime)
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
					if scenario != "valid" {
						assert.Empty(t, serial.payloads)
						assert.Nil(t, provides.Provider.Callback, "ineligible inventory must not register periodic collection")
						return
					}
					require.Len(t, serial.payloads, 1)
					require.NotNil(t, provides.Provider.Callback)
					provides.Provider.Callback(context.Background())
					require.Len(t, serial.payloads, 2)
					for index, data := range serial.payloads {
						var payload struct {
							Metadata map[string]interface{} `json:"agent_metadata"`
						}
						require.NoError(t, json.Unmarshal(data, &payload))
						assert.Equal(t, platform.resourceID, payload.Metadata["resource_id"])
						assert.Nil(t, payload.Metadata["parent_resource_id"], "no parent must be absent or JSON null")
						assert.Equal(t, "Test-Site", payload.Metadata["resource_name"])
						assert.Equal(t, platform.name, payload.Metadata["workload_type"])
						assert.Equal(t, "custom-dd-service", payload.Metadata["dd_service"])
						assert.Equal(t, []string{"startup", "periodic"}[index], payload.Metadata["report_reason"])
						assert.NotContains(t, payload.Metadata, "deployment_id")
					}
				})
			}
		})
	}
}
