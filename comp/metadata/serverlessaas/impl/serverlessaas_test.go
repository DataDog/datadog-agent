// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverlessaasimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

type panicHostname struct{ hostnameinterface.Component }

func (panicHostname) Get(context.Context) (string, error) {
	panic("hostname must not be collected during construction")
}

func aasEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WEBSITE_SITE_NAME", "My-App")
	t.Setenv("WEBSITE_OWNER_NAME", "sub-123+East US")
	t.Setenv("WEBSITE_RESOURCE_GROUP", "My-RG")
}

func TestConstructorDoesNotCollectHostname(t *testing.T) {
	aasEnv(t)

	assert.NotPanics(t, func() {
		NewComponent(Requires{
			Config: config.NewMockWithOverrides(t, map[string]any{
				"serverless.aas_extension_metadata_enabled": true,
				"enable_metadata_collection":                true,
				"inventories_enabled":                       true,
			}),
			Hostname:   panicHostname{},
			Log:        logmock.New(t),
			Serializer: serializermock.NewMetricSerializer(t),
		})
	})
}

func TestBuildFieldsFunctionAppAndSlot(t *testing.T) {
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "dotnet")
	t.Setenv("WEBSITE_SLOT_NAME", "Staging")
	aasEnv(t)

	fields := buildFields()

	require.NotNil(t, fields)
	assert.Equal(t, "azure_function", fields["workload_type"])
	assert.Equal(
		t,
		"/subscriptions/sub-123/resourcegroups/my-rg/providers/microsoft.web/sites/my-app/slots/staging",
		fields["resource_id"],
	)
}

func TestWorkloadTypeFunctionAppWhenRuntimeIsEmpty(t *testing.T) {
	t.Setenv("WEBSITE_SITE_NAME", "My-App")
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "")

	assert.Equal(t, "azure_function", workloadType())
}

func TestAppServiceRuntime(t *testing.T) {
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "")
	t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "")

	t.Setenv("WEBSITE_STACK", "JAVA")
	assert.Equal(t, "Java", appServiceRuntime())

	t.Setenv("WEBSITE_STACK", "")
	t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "~22")
	assert.Equal(t, "Node.js", appServiceRuntime())

	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "python")
	assert.Equal(t, "python", appServiceRuntime())
}
