// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
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

func TestInventoryUnsupportedWorkloadsDisabled(t *testing.T) {
	for _, platform := range []struct {
		name string
		env  map[string]string
	}{
		{
			name: "azure_container_app",
			env: map[string]string{
				"CONTAINER_APP_NAME": "test-app", "CONTAINER_APP_REVISION": "test-revision",
				"DD_AZURE_SUBSCRIPTION_ID": "test-subscription", "DD_AZURE_RESOURCE_GROUP": "test-group",
			},
		},
		{
			name: "azure_app_service",
			env: map[string]string{
				"WEBSITE_STACK": "NODE", "WEBSITE_SITE_NAME": "test-site",
				"WEBSITE_OWNER_NAME": "test-subscription+webspace", "WEBSITE_RESOURCE_GROUP": "test-group",
			},
		},
		{
			name: "azure_function",
			env: map[string]string{
				"WEBSITE_STACK": "NODE", "WEBSITE_SITE_NAME": "test-function", "FUNCTIONS_WORKER_RUNTIME": "node",
				"WEBSITE_OWNER_NAME": "test-subscription+webspace", "WEBSITE_RESOURCE_GROUP": "test-group",
			},
		},
		{
			name: "microvm",
			env:  map[string]string{"AWS_LAMBDA_MICROVM_IMAGE_ARN": "arn:aws:lambda:us-east-1:123456789012:microvm-image:test-image"},
		},
	} {
		t.Run(platform.name, func(t *testing.T) {
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
			require.NotEqual(t, "local", service.GetOrigin())
			require.False(t, service.CanCollectInventory())
			assert.Equal(t, cloudservice.InventoryData{}, service.GetInventoryData())

			conf := coreconfig.NewMock(t)
			for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
				conf.Set(key, true, configmodel.SourceAgentRuntime)
			}
			configureInventory(service)
			assert.False(t, conf.GetBool("inventories_enabled"))

			serial := &inventoryRecordingSerializer{}
			hostname, _ := hostnamemock.NewMock("inventory-test")
			provides := inventoryagentimpl.NewComponent(inventoryagentimpl.Requires{
				Config: conf, Log: logmock.New(t), Hostname: hostname, Serializer: serial, Capabilities: serverlessInitInventory.NewCapabilities(),
			})
			serverlessInitInventory.Inject(provides.Comp, service, mode.Conf{SidecarMode: true}, conf, nil)
			serverlessInitInventory.Submit(provides.Comp, conf)
			assert.Empty(t, serial.payloads, "an enabled flag must not submit unsupported inventory")
			assert.Empty(t, provides.Comp.Get(), "the disabled component must not retain injected inventory")
			assert.Nil(t, provides.Provider.Callback, "unsupported workloads must not register periodic collection")
		})
	}
}
