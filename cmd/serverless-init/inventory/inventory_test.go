// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package inventory

import (
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

func TestInjectSetsFieldsWithoutSubmitting(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", true, model.SourceAgentRuntime)
	ia := newFakeComponent()

	Inject(ia, &cloudservice.MicroVM{}, mode.Conf{}, conf, map[string]string{})

	assert.Equal(t, serverlessInitFlavor, ia.fields["flavor"])
	assert.Zero(t, ia.submits, "Inject must not enqueue a payload")
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
