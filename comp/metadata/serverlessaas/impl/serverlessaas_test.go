// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverlessaasimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/metadata/agentmetadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

type recordingSerializer struct {
	serializer.MetricSerializer
	payloads []*agentmetadata.Payload
}

func (s *recordingSerializer) SendMetadata(payload marshaler.JSONMarshaler) error {
	s.payloads = append(s.payloads, payload.(*agentmetadata.Payload))
	return nil
}

type fakeHostname struct{ hostnameinterface.Component }

func (fakeHostname) Get(context.Context) (string, error) { return "", nil }
func (fakeHostname) GetWithProvider(context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{}, nil
}

type panicHostname struct{ hostnameinterface.Component }

func (panicHostname) Get(context.Context) (string, error) {
	panic("hostname must not be collected during construction")
}

func aasEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WEBSITE_SITE_NAME", "My-App")
	t.Setenv("WEBSITE_OWNER_NAME", "sub-123+East US")
	t.Setenv("WEBSITE_RESOURCE_GROUP", "My-RG")
	t.Setenv("REGION_NAME", "East US")
}

func enabledConfig(t *testing.T) config.Component {
	t.Helper()
	return config.NewMockWithOverrides(t, map[string]any{
		"serverless.aas_extension_metadata_enabled": true,
		"enable_metadata_collection":                true,
		"inventories_enabled":                       true,
		"inventories_configuration_enabled":         false,
		"inventories_first_run_delay":               3600,
		"inventories_min_interval":                  60,
		"inventories_max_interval":                  600,
		"env":                                       "staging",
		"site":                                      "datad0g.com",
	})
}

func newTestComponent(t *testing.T, conf config.Component) (Provides, *recordingSerializer) {
	t.Helper()
	serializer := &recordingSerializer{}
	provides := NewComponent(Requires{
		Config:     conf,
		Hostname:   fakeHostname{},
		Log:        logmock.New(t),
		Serializer: serializer,
	})
	return provides, serializer
}

func TestGlobalInventoryGatesAreRespected(t *testing.T) {
	aasEnv(t)
	for _, key := range []string{"enable_metadata_collection", "inventories_enabled"} {
		t.Run(key, func(t *testing.T) {
			conf := config.NewMockWithOverrides(t, map[string]any{
				"enable_metadata_collection":                key != "enable_metadata_collection",
				"inventories_enabled":                       key != "inventories_enabled",
				"serverless.aas_extension_metadata_enabled": true,
			})
			provides, _ := newTestComponent(t, conf)
			assert.Nil(t, provides.Provider.Callback)
			assert.Nil(t, provides.Comp.(*serverlessAASComp).fields)
		})
	}
}

func TestAASMetadataDisabledByDefault(t *testing.T) {
	aasEnv(t)
	t.Setenv("DD_SERVERLESS_AAS_EXTENSION_METADATA_ENABLED", "")
	conf := config.NewMockWithOverrides(t, map[string]any{
		"enable_metadata_collection": true,
		"inventories_enabled":        true,
	})
	provides, serializer := newTestComponent(t, conf)
	assert.Nil(t, provides.Provider.Callback)
	assert.Nil(t, provides.Comp.(*serverlessAASComp).fields)
	assert.Empty(t, serializer.payloads)
}

func TestConstructorDoesNotCollectHostname(t *testing.T) {
	aasEnv(t)

	assert.NotPanics(t, func() {
		NewComponent(Requires{
			Config:     enabledConfig(t),
			Hostname:   panicHostname{},
			Log:        logmock.New(t),
			Serializer: &recordingSerializer{},
		})
	})
}

func TestFirstCompletePayloadIsImmediate(t *testing.T) {
	t.Setenv("DD_AAS_DOTNET_EXTENSION_VERSION", "3.12.0")
	t.Setenv("DD_SERVICE", "checkout")
	t.Setenv("DD_VERSION", "2026.10")
	aasEnv(t)
	conf := enabledConfig(t)
	provides, serializer := newTestComponent(t, conf)

	interval := provides.Provider.Callback(t.Context())

	assert.Equal(t, time.Duration(3600), conf.GetDuration("inventories_first_run_delay"))
	assert.Equal(t, time.Minute, interval)
	require.Len(t, serializer.payloads, 1)
	payload := serializer.payloads[0]
	assert.NotEmpty(t, payload.UUID)
	assert.Empty(t, payload.Hostname)
	assert.NotEmpty(t, payload.Metadata["agent_version"])
	for key, expected := range map[string]any{
		"flavor":                aasMetadataFlavor,
		"report_reason":         reportReasonStartup,
		"resource_id":           "/subscriptions/sub-123/resourcegroups/my-rg/providers/microsoft.web/sites/my-app",
		"resource_name":         "My-App",
		"workload_type":         workloadTypeAzureAppService,
		"region":                "East US",
		"azure_subscription_id": "sub-123",
		"azure_resource_group":  "My-RG",
		"runtime":               ".NET",
		"extension_version":     "3.12.0",
		"dd_env":                "staging",
		"dd_site":               "datad0g.com",
		"dd_service":            "checkout",
		"dd_version":            "2026.10",
	} {
		assert.Equal(t, expected, payload.Metadata[key], key)
	}
}

func TestSuccessfulSubmissionChangesReportReason(t *testing.T) {
	aasEnv(t)
	provides, serializer := newTestComponent(t, enabledConfig(t))
	component := provides.Comp.(*serverlessAASComp)

	provides.Provider.Callback(t.Context())
	require.Len(t, serializer.payloads, 1)
	assert.Equal(t, reportReasonStartup, serializer.payloads[0].Metadata["report_reason"])

	component.payload.Refresh()
	component.payload.LastCollect = component.payload.LastCollect.Add(-component.payload.MinInterval)
	provides.Provider.Callback(t.Context())
	require.Len(t, serializer.payloads, 2)
	assert.Equal(t, reportReasonPeriodic, serializer.payloads[1].Metadata["report_reason"])
	assert.Equal(t, reportReasonStartup, serializer.payloads[0].Metadata["report_reason"], "later collections must not mutate queued payloads")
}

func TestStaticFieldsAreCollectedOnce(t *testing.T) {
	aasEnv(t)
	provides, serializer := newTestComponent(t, enabledConfig(t))
	t.Setenv("WEBSITE_SITE_NAME", "changed-after-construction")
	t.Setenv("REGION_NAME", "changed-after-construction")

	provides.Provider.Callback(t.Context())

	require.Len(t, serializer.payloads, 1)
	assert.Equal(t, "My-App", serializer.payloads[0].Metadata["resource_name"])
	assert.Equal(t, "East US", serializer.payloads[0].Metadata["region"])
}

func TestBuildFieldsFunctionAppAndSlot(t *testing.T) {
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "dotnet")
	t.Setenv("WEBSITE_SLOT_NAME", "Staging")
	aasEnv(t)

	fields := buildFields()

	require.NotNil(t, fields)
	assert.Equal(t, workloadTypeAzureFunction, fields["workload_type"])
	assert.Equal(
		t,
		"/subscriptions/sub-123/resourcegroups/my-rg/providers/microsoft.web/sites/my-app/slots/staging",
		fields["resource_id"],
	)
}

func TestWorkloadTypeFunctionAppWhenRuntimeIsEmpty(t *testing.T) {
	t.Setenv("WEBSITE_SITE_NAME", "My-App")
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "")

	assert.Equal(t, workloadTypeAzureFunction, workloadType())
}

func TestProductionSlotIsOmitted(t *testing.T) {
	t.Setenv("WEBSITE_SLOT_NAME", "Production")
	aasEnv(t)

	fields := buildFields()

	require.NotNil(t, fields)
	assert.Equal(
		t,
		"/subscriptions/sub-123/resourcegroups/my-rg/providers/microsoft.web/sites/my-app",
		fields["resource_id"],
	)
}

func TestAppServiceRuntime(t *testing.T) {
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "")
	t.Setenv("WEBSITE_STACK", "")
	t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "")
	assert.Equal(t, ".NET", appServiceRuntime())

	t.Setenv("WEBSITE_STACK", "JAVA")
	assert.Equal(t, "Java", appServiceRuntime())

	t.Setenv("WEBSITE_STACK", "")
	t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "~22")
	assert.Equal(t, "Node.js", appServiceRuntime())

	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "python")
	assert.Equal(t, "python", appServiceRuntime())
}
