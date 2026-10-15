// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package serverlessinitimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	serverlessinit "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// fakeInventoryAgent records the calls this component makes on the
// inventoryagent payload, in order.
type fakeInventoryAgent struct {
	fields  map[string]interface{}
	batches []map[string]interface{}
	calls   []string
}

func newFakeInventoryAgent() *fakeInventoryAgent {
	return &fakeInventoryAgent{fields: map[string]interface{}{}}
}

func (f *fakeInventoryAgent) Set(name string, value interface{}) {
	f.calls = append(f.calls, "Set:"+name)
	f.fields[name] = value
}

func (f *fakeInventoryAgent) SetBatch(fields map[string]interface{}) {
	f.calls = append(f.calls, "SetBatch")
	f.batches = append(f.batches, fields)
	for name, value := range fields {
		f.fields[name] = value
	}
}

func (f *fakeInventoryAgent) SetReady(bool) {
	f.calls = append(f.calls, "SetReady")
}

func (f *fakeInventoryAgent) Get() map[string]interface{} { return f.fields }

func (f *fakeInventoryAgent) Submit() { f.calls = append(f.calls, "Submit") }

type fakeProvider struct {
	data     serverlessinit.InventoryData
	eligible bool
}

func (p *fakeProvider) CanCollectInventory() bool { return p.eligible }

func (p *fakeProvider) GetInventoryData() serverlessinit.InventoryData { return p.data }

type hookRecorder struct {
	hooks []compdef.Hook
}

func (h *hookRecorder) Append(hook compdef.Hook) { h.hooks = append(h.hooks, hook) }

func newTestComponent(t *testing.T, inventoryEnabled bool, provider *fakeProvider, params serverlessinit.Params) (serverlessinit.Component, *fakeInventoryAgent, *hookRecorder, *serverlessinit.PayloadUUID) {
	conf := configmock.New(t)
	if inventoryEnabled {
		conf.Set("serverless.inventory_enabled", true, model.SourceAgentRuntime)
	}
	ia := newFakeInventoryAgent()
	lc := &hookRecorder{}
	uuid := serverlessinit.NewPayloadUUID()
	comp := NewComponent(Requires{
		Lc:             lc,
		Log:            logmock.New(t),
		Config:         conf,
		Params:         params,
		InventoryAgent: ia,
		Provider:       provider,
		UUID:           uuid,
	}).Comp
	return comp, ia, lc, uuid
}

func TestNewComponentConstructionIdentity(t *testing.T) {
	t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", "")
	provider := &fakeProvider{
		eligible: true,
		data: serverlessinit.InventoryData{
			ResourceID:   "test-resource",
			ResourceName: "test-app",
			WorkloadType: "azure_app_service",
		},
	}

	_, ia, lc, _ := newTestComponent(t, true, provider, serverlessinit.Params{
		Tags: map[string]string{"env": "test-env", "service": "test-service"},
	})

	// Publication protocol: the field set is applied while closed and opened
	// only once complete.
	assert.Equal(t, []string{"SetReady", "SetBatch", "SetReady"}, ia.calls)
	require.Len(t, ia.batches, 1)
	batch := ia.batches[0]
	assert.Equal(t, "test-resource", batch["resource_id"])
	assert.Equal(t, "azure_app_service", batch["workload_type"])
	assert.Equal(t, serverlessInitFlavor, batch["flavor"])
	assert.Equal(t, reportReasonStartup, batch["report_reason"])
	assert.Equal(t, "in-container", batch["deployment_model"])
	assert.Equal(t, "test-env", batch["dd_env"])
	assert.NotContains(t, ia.calls, "Submit", "the startup submission waits for the lifecycle hook")
	require.Len(t, lc.hooks, 1)
	require.NoError(t, lc.hooks[0].OnStart(context.Background()))
	assert.Equal(t, []string{"SetReady", "SetBatch", "SetReady", "Submit", "Set:report_reason"}, ia.calls)
	assert.Equal(t, reportReasonPeriodic, ia.fields["report_reason"], "runner submissions after the startup payload report periodic")
}

func TestNewComponentPayloadUUIDRotation(t *testing.T) {
	provider := &fakeProvider{eligible: true, data: serverlessinit.InventoryData{ResourceID: "instance-a"}}
	comp, ia, _, uuid := newTestComponent(t, true, provider, serverlessinit.Params{})
	first := uuid.Get()
	require.NotEmpty(t, first)

	// A same-identity resume preserves the uuid; a new identity rotates it.
	comp.InventoryIdentityArrived()
	assert.Equal(t, first, uuid.Get(), "same identity must not rotate the uuid")
	require.Len(t, ia.batches, 2)

	provider.data = serverlessinit.InventoryData{ResourceID: "instance-b"}
	comp.InventoryIdentityArrived()
	assert.NotEqual(t, first, uuid.Get(), "a changed identity must rotate the uuid")
}

func TestNewComponentIdentityEvent(t *testing.T) {
	t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", "")
	provider := &fakeProvider{eligible: true}

	comp, ia, lc, _ := newTestComponent(t, true, provider, serverlessinit.Params{
		Sidecar: true,
		Tags:    map[string]string{"service": "test-service"},
	})

	assert.Empty(t, ia.calls, "no field may be applied before resource identity exists")
	assert.Empty(t, lc.hooks, "no startup submission may be scheduled without identity")

	provider.data = serverlessinit.InventoryData{
		ResourceID:   "late-resource",
		WorkloadType: "aws_lambda_microvm",
	}
	comp.InventoryIdentityArrived()

	assert.Equal(t, []string{"SetReady", "SetBatch", "SetReady", "Submit", "Set:report_reason"}, ia.calls)
	require.Len(t, ia.batches, 1)
	assert.Equal(t, "late-resource", ia.batches[0]["resource_id"])
	assert.Equal(t, "sidecar", ia.batches[0]["deployment_model"])
	assert.Equal(t, reportReasonPeriodic, ia.fields["report_reason"])
}

func TestNewComponentIdentityEventWithoutIdentity(t *testing.T) {
	provider := &fakeProvider{eligible: true}

	comp, ia, _, _ := newTestComponent(t, true, provider, serverlessinit.Params{})

	comp.InventoryIdentityArrived()
	assert.Empty(t, ia.calls, "an identity event that carries no resource id must not publish")
}

func TestNewComponentGatedOff(t *testing.T) {
	for _, tt := range []struct {
		name              string
		inventoryEnabled  bool
		providerEligible  bool
		identityAtStartup bool
	}{
		{name: "inventory disabled", providerEligible: true, identityAtStartup: true},
		{name: "provider cannot collect", inventoryEnabled: true},
		{name: "inventory disabled and provider cannot collect"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeProvider{eligible: tt.providerEligible}
			if tt.identityAtStartup {
				provider.data = serverlessinit.InventoryData{ResourceID: "test-resource"}
			}

			comp, ia, lc, _ := newTestComponent(t, tt.inventoryEnabled, provider, serverlessinit.Params{})

			assert.Empty(t, ia.calls)
			assert.Empty(t, lc.hooks)
			comp.InventoryIdentityArrived()
			assert.Empty(t, ia.calls, "an ineligible component must stay inert")
		})
	}
}
