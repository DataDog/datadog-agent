// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package inventory

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// fakeComponent records Set calls and Submit invocations so tests can assert on
// the fields the inventory package layers onto the shared inventoryagent
// component.
type fakeComponent struct {
	fields  map[string]interface{}
	submits int
}

func newFakeComponent() *fakeComponent {
	return &fakeComponent{fields: map[string]interface{}{}}
}

func (f *fakeComponent) Set(name string, value interface{}) { f.fields[name] = value }
func (f *fakeComponent) Get() map[string]interface{}        { return f.fields }
func (f *fakeComponent) Submit()                            { f.submits++ }

type inventoryCloudService struct {
	cloudservice.CloudService
	data cloudservice.InventoryData
}

func (s inventoryCloudService) GetInventoryData() cloudservice.InventoryData { return s.data }

func TestBuildFieldsMissingValues(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("site", "", model.SourceAgentRuntime)
	service := inventoryCloudService{data: cloudservice.InventoryData{
		ResourceID:   "test-resource",
		ResourceName: "test-app",
		WorkloadType: "azure_app_service",
	}}

	fields := buildFields(service, mode.Conf{SidecarMode: true}, conf, nil)

	for _, key := range []string{
		"parent_resource_id", "region", "gcp_project_id", "aws_account_id",
		"azure_subscription_id", "azure_resource_group", "runtime",
		"dd_env", "dd_site", "dd_version", "dd_service",
	} {
		assert.Contains(t, fields, key, "missing fields must still be passed to Set to clear cached values")
		assert.Nil(t, fields[key], key)
	}
	assert.Equal(t, service.data.ResourceID, fields["resource_id"])
	assert.Equal(t, service.data.ResourceName, fields["resource_name"])
	assert.Equal(t, service.data.WorkloadType, fields["workload_type"])
	assert.Equal(t, "sidecar", fields["deployment_model"])
	assert.NotContains(t, fields, "wrapped_command")
	assert.NotContains(t, fields, "deployment_id")
	assert.NotContains(t, fields, "tags")
}

func TestBuildFieldsPreservesPopulatedValues(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("site", "datadoghq.eu", model.SourceAgentRuntime)
	service := inventoryCloudService{data: cloudservice.InventoryData{
		ResourceID:          "test-resource",
		ResourceName:        "test-app",
		WorkloadType:        "azure_app_service",
		ParentResourceID:    "test-parent",
		Region:              "test-region",
		GCPProjectID:        "test-project",
		AWSAccountID:        "123456789012",
		AzureSubscriptionID: "test-subscription",
		AzureResourceGroup:  "test-group",
		Runtime:             "python",
	}}

	fields := buildFields(service, mode.Conf{SidecarMode: true}, conf, map[string]string{
		"env": "test-env", "service": "test-service", "version": "test-version",
	})

	for key, expected := range map[string]string{
		"resource_id": "test-resource", "resource_name": "test-app", "workload_type": "azure_app_service",
		"parent_resource_id": "test-parent", "region": "test-region", "gcp_project_id": "test-project",
		"aws_account_id": "123456789012", "azure_subscription_id": "test-subscription", "azure_resource_group": "test-group",
		"runtime": "python", "dd_env": "test-env", "dd_service": "test-service", "dd_version": "test-version", "dd_site": "datadoghq.eu",
	} {
		assert.Equal(t, expected, fields[key], key)
	}
}

func TestBuildFieldsPreservesNonemptyStrings(t *testing.T) {
	conf := configmock.New(t)
	for _, value := range []string{"unknown", "null", " ", " python "} {
		t.Run(value, func(t *testing.T) {
			service := inventoryCloudService{data: cloudservice.InventoryData{Runtime: value}}
			fields := buildFields(service, mode.Conf{SidecarMode: true}, conf, nil)
			assert.Equal(t, value, fields["runtime"])
		})
	}
}

func TestBuildFieldsWrappedCommand(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
	conf := configmock.New(t)

	for _, scenario := range []struct {
		name    string
		args    []string
		sidecar bool
	}{
		{name: "no command", args: []string{"serverless-init"}},
		{name: "sidecar", args: []string{"serverless-init", "python", "app.py"}, sidecar: true},
		{name: "wrapped command", args: []string{"serverless-init", "python", "app.py", "--password=secret"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			os.Args = scenario.args
			fields := buildFields(inventoryCloudService{}, mode.Conf{SidecarMode: scenario.sidecar}, conf, nil)
			if scenario.sidecar || len(scenario.args) == 1 {
				assert.NotContains(t, fields, "wrapped_command")
				return
			}
			assert.Contains(t, fields["wrapped_command"], "python app.py")
			assert.Contains(t, fields["wrapped_command"], "--password=")
			assert.NotContains(t, fields["wrapped_command"], "secret")
			assert.Equal(t, "in-container", fields["deployment_model"])
		})
	}
}

func TestInjectSetsFieldsWithoutSubmitting(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", true, model.SourceAgentRuntime)
	ia := newFakeComponent()

	Inject(ia, &cloudservice.MicroVM{}, mode.Conf{}, conf, map[string]string{})

	assert.Equal(t, serverlessInitFlavor, ia.fields["flavor"])
	assert.Zero(t, ia.submits, "Inject must not enqueue a payload")
}

// Pins where each Unified Service Tagging field is sourced from: env, service,
// and version track the tag map so inventory agrees with the tags on this
// container's telemetry, while site is not a tag and comes from the config.
func TestInjectReportsUnifiedServiceTaggingFromTagMap(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", true, model.SourceAgentRuntime)
	conf.Set("site", "datadoghq.eu", model.SourceAgentRuntime)
	conf.Set("env", "config-env", model.SourceAgentRuntime)
	ia := newFakeComponent()

	Inject(ia, &cloudservice.MicroVM{}, mode.Conf{}, conf, map[string]string{
		"env":     "tag-env",
		"service": "my-service",
		"version": "1.2.3",
	})

	assert.Equal(t, "tag-env", ia.fields["dd_env"],
		"env must come from the tag map, which is lowercased and honors DD_TAGS")
	assert.Equal(t, "my-service", ia.fields["dd_service"])
	assert.Equal(t, "1.2.3", ia.fields["dd_version"])
	assert.Equal(t, "datadoghq.eu", ia.fields["dd_site"])
}

func TestInjectGatedOff(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", false, model.SourceAgentRuntime)
	ia := newFakeComponent()

	Inject(ia, &cloudservice.MicroVM{}, mode.Conf{}, conf, map[string]string{})

	assert.Empty(t, ia.fields, "no fields must be set when the ramp gate is off")
}

func TestSubmitEnqueuesWhenEnabled(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", true, model.SourceAgentRuntime)
	ia := newFakeComponent()

	Submit(ia, conf)

	assert.Equal(t, 1, ia.submits)
}

func TestSubmitGatedOff(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", false, model.SourceAgentRuntime)
	ia := newFakeComponent()

	Submit(ia, conf)

	assert.Zero(t, ia.submits, "Submit must not enqueue a payload when the ramp gate is off")
}

func TestSetResourceIDWhenEnabled(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", true, model.SourceAgentRuntime)
	ia := newFakeComponent()

	SetResourceID(ia, conf, "vm-abc123")

	assert.Equal(t, "vm-abc123", ia.fields["resource_id"])
}

func TestSetResourceIDGatedOff(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", false, model.SourceAgentRuntime)
	ia := newFakeComponent()

	SetResourceID(ia, conf, "vm-abc123")

	assert.Empty(t, ia.fields, "resource_id must not be set when the ramp gate is off")
}

// The MicroVM lifecycle server reports the stored instance id on /resume, which
// is empty when no /run delivered one; the image ARN that Inject derived must
// survive that.
func TestSetResourceIDIgnoresEmptyID(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", true, model.SourceAgentRuntime)
	ia := newFakeComponent()
	ia.Set("resource_id", "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image")

	SetResourceID(ia, conf, "")

	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image", ia.fields["resource_id"])
}

func TestInjectOmitsDeprecatedDeploymentID(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", true, model.SourceAgentRuntime)
	ia := newFakeComponent()

	Inject(ia, &cloudservice.MicroVM{}, mode.Conf{}, conf, map[string]string{})

	assert.NotContains(t, ia.fields, "deployment_id")
}

func TestNewCapabilitiesReportsOneUUIDForProcessLifetime(t *testing.T) {
	caps := NewCapabilities()

	assert.True(t, caps.SkipCrossProcessEnrichment)
	assert.NotEmpty(t, caps.PayloadUUID())
	assert.Equal(t, caps.PayloadUUID(), caps.PayloadUUID())
}

func TestNewCapabilitiesDistinctPerProcess(t *testing.T) {
	assert.NotEqual(t, NewCapabilities().PayloadUUID(), NewCapabilities().PayloadUUID())
}

func TestNewInstanceUUIDResolvesBeforeAnyInstance(t *testing.T) {
	u := NewInstanceUUID()

	assert.NotEmpty(t, u.Resolve(), "a payload built before the first transition still needs a uuid")
}

func TestSetInstanceRotatesUUIDForNewInstance(t *testing.T) {
	u := NewInstanceUUID()
	snapshotUUID := u.Resolve()

	u.SetInstance("vm-abc123")

	assert.NotEqual(t, snapshotUUID, u.Resolve(),
		"a restored instance must not keep reporting the snapshot's uuid")
}

func TestSetInstanceKeepsUUIDForSameInstance(t *testing.T) {
	u := NewInstanceUUID()
	u.SetInstance("vm-abc123")
	runUUID := u.Resolve()

	u.SetInstance("vm-abc123")

	assert.Equal(t, runUUID, u.Resolve(),
		"a later transition of the same instance must report the same uuid")
}

func TestSetInstanceRotatesUUIDPerInstance(t *testing.T) {
	first := NewInstanceUUID()
	first.SetInstance("vm-abc123")
	second := NewInstanceUUID()
	second.SetInstance("vm-def456")

	assert.NotEqual(t, first.Resolve(), second.Resolve(),
		"instances restored from one snapshot must report distinct uuids")
}

func TestSetInstanceIgnoresEmptyID(t *testing.T) {
	u := NewInstanceUUID()
	before := u.Resolve()

	u.SetInstance("")

	assert.Equal(t, before, u.Resolve(), "an absent id carries no identity to adopt")
}

func TestSetInstanceOnNilReceiver(t *testing.T) {
	var u *InstanceUUID

	assert.NotPanics(t, func() { u.SetInstance("vm-abc123") },
		"non-MicroVM platforms leave this nil and call it unconditionally")
}

// Pins the invariant that makes a mutex necessary rather than two atomics: uuid
// and instanceID must move together.
func TestInstanceUUIDConcurrentSetInstanceRotatesOnce(t *testing.T) {
	u := NewInstanceUUID()
	snapshotUUID := u.Resolve()

	const goroutines = 100
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(2)
		go func() {
			defer wg.Done()
			u.SetInstance("vm-abc123")
		}()
		go func() {
			defer wg.Done()
			u.Resolve()
		}()
	}
	wg.Wait()

	rotated := u.Resolve()
	assert.NotEqual(t, snapshotUUID, rotated)

	u.SetInstance("vm-abc123")
	assert.Equal(t, rotated, u.Resolve(),
		"one instance must settle on one uuid however many transitions it reports")
}

func TestNewInstanceCapabilitiesResolvesPerPayload(t *testing.T) {
	u := NewInstanceUUID()
	caps := NewInstanceCapabilities(u)

	assert.True(t, caps.SkipCrossProcessEnrichment)
	assert.Equal(t, u.Resolve(), caps.PayloadUUID())

	u.SetInstance("vm-abc123")

	assert.Equal(t, u.Resolve(), caps.PayloadUUID(),
		"the uuid must resolve per payload, not be captured once")
}
