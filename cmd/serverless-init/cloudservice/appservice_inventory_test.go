// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestAppServiceInventoryOffPreservesTelemetry(t *testing.T) {
	for _, worker := range []string{"", "python"} {
		t.Run("worker="+worker, func(t *testing.T) {
			conf := configmock.New(t)
			conf.Set("serverless.inventory_enabled", false, model.SourceAgentRuntime)
			t.Setenv("WEBSITE_SITE_NAME", "Test-Site")
			t.Setenv("WEBSITE_OWNER_NAME", "Test-Subscription+webspace")
			t.Setenv("WEBSITE_RESOURCE_GROUP", "Test-Group")
			t.Setenv("REGION_NAME", "eastus")
			t.Setenv("WEBSITE_STACK", "NODE")
			t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "~18")
			t.Setenv("COMPUTERNAME", "Test-Instance")
			t.Setenv("FUNCTIONS_WORKER_RUNTIME", worker)
			if worker == "" {
				require.NoError(t, os.Unsetenv("FUNCTIONS_WORKER_RUNTIME"))
			}
			service := &AppService{}
			tags := service.GetTags()
			assert.Equal(t, "/subscriptions/Test-Subscription/resourcegroups/Test-Group/providers/microsoft.web/sites/Test-Site", tags["aas.resource.id"])
			if worker == "" {
				assert.Equal(t, "Node.js", tags["aas.environment.runtime"])
			} else {
				assert.Equal(t, worker, tags["aas.environment.runtime"])
			}
			enhanced := service.GetEnhancedMetricTags(tags)
			assert.Equal(t, map[string]string{
				"name": "Test-Site", "origin": "appservice", "region": "eastus",
				"resource_group": "Test-Group", "subscription_id": "Test-Subscription",
			}, enhanced.Base)
			assert.Equal(t, "Test-Instance", enhanced.Usage["instance"])

			t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", "InventoryOnly")
			service.GetInventoryData()
			assert.Equal(t, tags, service.GetTags())
			assert.Equal(t, enhanced, service.GetEnhancedMetricTags(service.GetTags()))
			assert.False(t, conf.GetBool("serverless.inventory_enabled"))
		})
	}
}
