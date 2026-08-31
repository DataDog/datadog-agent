// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package inventory

import (
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

func TestSetDeploymentIDWhenEnabled(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", true, model.SourceAgentRuntime)
	ia := newFakeComponent()

	SetDeploymentID(ia, conf, "vm-abc123")

	assert.Equal(t, "vm-abc123", ia.fields[serverlessFieldPrefix+"deployment_id"])
}

func TestSetDeploymentIDGatedOff(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", false, model.SourceAgentRuntime)
	ia := newFakeComponent()

	SetDeploymentID(ia, conf, "vm-abc123")

	assert.Empty(t, ia.fields, "deployment_id must not be set when the ramp gate is off")
}
