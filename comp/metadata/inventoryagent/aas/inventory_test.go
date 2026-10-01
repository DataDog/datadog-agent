// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aas

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// fakeComponent records Set calls and Submit invocations.
type fakeComponent struct {
	fields    map[string]interface{}
	submits   int
	submitted chan struct{}
}

func newFake() *fakeComponent {
	return &fakeComponent{
		fields:    map[string]interface{}{},
		submitted: make(chan struct{}, 10),
	}
}

func (f *fakeComponent) Set(name string, value interface{}) { f.fields[name] = value }
func (f *fakeComponent) Get() map[string]interface{}        { return f.fields }
func (f *fakeComponent) Submit() {
	f.submits++
	f.submitted <- struct{}{}
}

// aasEnv sets the minimum AAS environment variables needed to produce a valid
// resource_id and cleans them up after the test.
func aasEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WEBSITE_SITE_NAME", "My-App")
	t.Setenv("WEBSITE_OWNER_NAME", "sub-123+East US")
	t.Setenv("WEBSITE_RESOURCE_GROUP", "My-RG")
	t.Setenv("REGION_NAME", "East US")
}

func enableInventory(t *testing.T) {
	t.Helper()
	t.Setenv(envInventoryEnabled, "1")
}

func TestIsEnabledUsesInventoryGate(t *testing.T) {
	assert.False(t, IsEnabled())

	t.Setenv(envInventoryEnabled, "1")
	assert.True(t, IsEnabled())
}

func TestInjectGatedOff(t *testing.T) {
	aasEnv(t)
	conf := configmock.New(t)
	ia := newFake()

	ok := Inject(ia, conf)

	assert.False(t, ok)
	assert.Empty(t, ia.fields, "no fields must be set when gate is off")
}

func TestInjectSetsFieldsWebApp(t *testing.T) {
	enableInventory(t)
	t.Setenv("DD_AAS_DOTNET_EXTENSION_VERSION", "3.12.0")
	aasEnv(t)
	conf := configmock.New(t)
	conf.Set("env", "staging", model.SourceAgentRuntime)
	conf.Set("site", "datad0g.com", model.SourceAgentRuntime)
	ia := newFake()

	ok := Inject(ia, conf)

	assert.True(t, ok)
	assert.Equal(t, "serverless-extension", ia.fields["flavor"])
	assert.Equal(t, workloadTypeAzureAppService, ia.fields["workload_type"])
	assert.Equal(t, reportReasonStartup, ia.fields["report_reason"])
	assert.Equal(t, "staging", ia.fields["dd_env"])
	assert.Equal(t, "datad0g.com", ia.fields["dd_site"])
	assert.Equal(
		t,
		"/subscriptions/sub-123/resourcegroups/my-rg/providers/microsoft.web/sites/my-app",
		ia.fields["resource_id"],
	)
	assert.Equal(t, "My-App", ia.fields["resource_name"])
	assert.Equal(t, "East US", ia.fields["region"])
	assert.Contains(t, ia.fields, "extension_version", "extension_version must be set for serverless_aas_extension_agent")
	assert.Zero(t, ia.submits, "Inject must not call Submit")
}

func TestInjectSetsFunctionAppWorkloadType(t *testing.T) {
	enableInventory(t)
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "dotnet")
	aasEnv(t)
	conf := configmock.New(t)
	ia := newFake()

	ok := Inject(ia, conf)

	assert.True(t, ok)
	assert.Equal(t, workloadTypeAzureFunction, ia.fields["workload_type"])
}

func TestInjectUsesDeploymentSlotResourceID(t *testing.T) {
	enableInventory(t)
	aasEnv(t)
	t.Setenv("WEBSITE_SLOT_NAME", "Staging")
	conf := configmock.New(t)
	ia := newFake()

	ok := Inject(ia, conf)

	assert.True(t, ok)
	assert.Equal(
		t,
		"/subscriptions/sub-123/resourcegroups/my-rg/providers/microsoft.web/sites/my-app/slots/staging",
		ia.fields["resource_id"],
	)
}

func TestInjectOmitsProductionDeploymentSlot(t *testing.T) {
	enableInventory(t)
	aasEnv(t)
	t.Setenv("WEBSITE_SLOT_NAME", "Production")
	conf := configmock.New(t)
	ia := newFake()

	ok := Inject(ia, conf)

	assert.True(t, ok)
	assert.Equal(
		t,
		"/subscriptions/sub-123/resourcegroups/my-rg/providers/microsoft.web/sites/my-app",
		ia.fields["resource_id"],
	)
}

func TestInjectSkipsWhenResourceIDEmpty(t *testing.T) {
	enableInventory(t)
	// No WEBSITE_SITE_NAME / WEBSITE_OWNER_NAME / WEBSITE_RESOURCE_GROUP →
	// traceutil.GetAppServicesTags() returns an empty resource_id.
	conf := configmock.New(t)
	ia := newFake()

	ok := Inject(ia, conf)

	assert.False(t, ok)
	assert.Empty(t, ia.fields, "must not set any fields when resource_id cannot be derived")
}

func TestSubmitGatedOff(t *testing.T) {
	ia := newFake()
	Submit(ia)
	assert.Zero(t, ia.submits)
}

func TestSubmitEnqueues(t *testing.T) {
	enableInventory(t)
	ia := newFake()
	ia.fields["report_reason"] = reportReasonStartup

	Submit(ia)

	assert.Equal(t, 1, ia.submits)
	assert.Equal(t, reportReasonPeriodic, ia.fields["report_reason"])
}

func TestSubmitOnTicksStopsWithContext(t *testing.T) {
	enableInventory(t)
	ia := newFake()
	ticks := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		submitOnTicks(ctx, ia, ticks)
	}()

	ticks <- time.Now()
	select {
	case <-ia.submitted:
	case <-time.After(time.Second):
		require.FailNow(t, "periodic inventory submission did not run")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		require.FailNow(t, "periodic inventory submission did not stop")
	}

	assert.Equal(t, 1, ia.submits)
}

func TestWorkloadTypeDetection(t *testing.T) {
	assert.Equal(t, workloadTypeAzureAppService, workloadType())

	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "node")
	assert.Equal(t, workloadTypeAzureFunction, workloadType())
}
