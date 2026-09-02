// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// fakeComponent records Set calls and Submit invocations.
type fakeComponent struct {
	fields  map[string]interface{}
	submits int
}

func newFake() *fakeComponent { return &fakeComponent{fields: map[string]interface{}{}} }

func (f *fakeComponent) Set(name string, value interface{}) { f.fields[name] = value }
func (f *fakeComponent) Get() map[string]interface{}        { return f.fields }
func (f *fakeComponent) Submit()                            { f.submits++ }

// aasEnv sets the minimum AAS environment variables needed to produce a valid
// resource_id and cleans them up after the test.
func aasEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WEBSITE_SITE_NAME", "my-app")
	t.Setenv("WEBSITE_OWNER_NAME", "sub-123+East US")
	t.Setenv("WEBSITE_RESOURCE_GROUP", "my-rg")
	t.Setenv("REGION_NAME", "East US")
}

func TestInjectGatedOff(t *testing.T) {
	aasEnv(t)
	conf := configmock.New(t)
	ia := newFake()

	Inject(ia, conf)

	assert.Empty(t, ia.fields, "no fields must be set when gate is off")
}

func TestInjectSetsFieldsWebApp(t *testing.T) {
	t.Setenv(envInventoryEnabled, "1")
	aasEnv(t)
	conf := configmock.New(t)
	conf.Set("env", "staging", model.SourceAgentRuntime)
	conf.Set("site", "datad0g.com", model.SourceAgentRuntime)
	ia := newFake()

	Inject(ia, conf)

	assert.Equal(t, aasInventoryFlavor, ia.fields["flavor"])
	assert.Equal(t, workloadTypeAzureAppService, ia.fields["workload_type"])
	assert.Equal(t, reportReasonStartup, ia.fields["report_reason"])
	assert.Equal(t, "staging", ia.fields["dd_env"])
	assert.Equal(t, "datad0g.com", ia.fields["dd_site"])
	assert.NotEmpty(t, ia.fields["resource_id"])
	assert.Equal(t, "my-app", ia.fields["resource_name"])
	assert.Zero(t, ia.submits, "Inject must not call Submit")
}

func TestInjectSetsFunctionAppWorkloadType(t *testing.T) {
	t.Setenv(envInventoryEnabled, "1")
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "dotnet")
	aasEnv(t)
	conf := configmock.New(t)
	ia := newFake()

	Inject(ia, conf)

	assert.Equal(t, workloadTypeAzureFunction, ia.fields["workload_type"])
}

func TestInjectSkipsWhenResourceIDEmpty(t *testing.T) {
	t.Setenv(envInventoryEnabled, "1")
	// No WEBSITE_SITE_NAME / WEBSITE_OWNER_NAME / WEBSITE_RESOURCE_GROUP →
	// traceutil.GetAppServicesTags() returns an empty resource_id.
	conf := configmock.New(t)
	ia := newFake()

	Inject(ia, conf)

	assert.Empty(t, ia.fields, "must not set any fields when resource_id cannot be derived")
}

func TestSubmitGatedOff(t *testing.T) {
	ia := newFake()
	Submit(ia)
	assert.Zero(t, ia.submits)
}

func TestSubmitEnqueuesAndSwitchesToPeriodic(t *testing.T) {
	t.Setenv(envInventoryEnabled, "1")
	ia := newFake()

	Submit(ia)

	assert.Equal(t, 1, ia.submits)
	assert.Equal(t, reportReasonPeriodic, ia.fields["report_reason"],
		"report_reason must be switched to periodic after Submit so the runner uses it")
}

func TestWorkloadTypeDetection(t *testing.T) {
	assert.Equal(t, workloadTypeAzureAppService, workloadType())

	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "node")
	assert.Equal(t, workloadTypeAzureFunction, workloadType())
}
