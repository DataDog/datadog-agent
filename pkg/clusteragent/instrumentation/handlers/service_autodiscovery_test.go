// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

func newServiceHandler() *ServiceAutodiscoveryHandler {
	return NewServiceAutodiscoveryHandler(&Deps{
		ServiceCheckTemplateStore: NewServiceCheckTemplateStore(),
	})
}

func newServiceCR(name, namespace, serviceName string, checks []datadoghq.DatadogInstrumentationCheckConfig) *datadoghq.DatadogInstrumentation {
	return newCR(name, namespace, "Service", serviceName, checks)
}

func TestServiceHandler_Name(t *testing.T) {
	h := newServiceHandler()
	assert.Equal(t, "service-autodiscovery", h.Name())
}

func TestServiceHandler_HasSection(t *testing.T) {
	tests := []struct {
		name     string
		cr       *datadoghq.DatadogInstrumentation
		expected bool
	}{
		{
			name:     "nil CR",
			cr:       nil,
			expected: false,
		},
		{
			name:     "no checks",
			cr:       newServiceCR("test", "default", "my-svc", nil),
			expected: false,
		},
		{
			name: "with checks",
			cr: newServiceCR("test", "default", "my-svc", []datadoghq.DatadogInstrumentationCheckConfig{
				{Integration: "redisdb"},
			}),
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newServiceHandler()
			assert.Equal(t, tt.expected, h.HasSection(tt.cr))
		})
	}
}

func TestServiceHandler_SupportsTarget(t *testing.T) {
	tests := []struct {
		kind     string
		expected bool
	}{
		{"Service", true},
		{"Deployment", false},
		{"DaemonSet", false},
		{"StatefulSet", false},
		{"CronJob", false},
		{"Job", false},
		{"ReplicaSet", false},
		{"Pod", false},
		{"", false},
	}
	h := newServiceHandler()
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			ref := autoscalingv2.CrossVersionObjectReference{Kind: tt.kind, Name: "test"}
			assert.Equal(t, tt.expected, h.SupportsTarget(ref))
		})
	}
}

func TestServiceHandler_Validate(t *testing.T) {
	tests := []struct {
		name           string
		cr             *datadoghq.DatadogInstrumentation
		expectErrCount int
		expectField    string
	}{
		{
			name:           "nil CR returns nil",
			cr:             nil,
			expectErrCount: 0,
		},
		{
			name: "valid check",
			cr: newServiceCR("test", "default", "my-svc", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:    "redisdb",
					ContainerImage: []string{"redis"},
					Instances:      []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 0,
		},
		{
			name: "empty integration name",
			cr: newServiceCR("test", "default", "my-svc", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:    "",
					ContainerImage: []string{"redis"},
					Instances:      []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].integration",
		},
		{
			name: "no instances or logs",
			cr: newServiceCR("test", "default", "my-svc", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:    "redisdb",
					ContainerImage: []string{"redis"},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].instances",
		},
		{
			name: "no container image",
			cr: newServiceCR("test", "default", "my-svc", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration: "redisdb",
					Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].containerImage",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newServiceHandler()
			errs := h.Validate(tt.cr)
			assert.Len(t, errs, tt.expectErrCount)
			if tt.expectField != "" && len(errs) > 0 {
				assert.Equal(t, tt.expectField, errs[0].Field)
			}
			for _, e := range errs {
				assert.Equal(t, checksReadyConditionType, e.Type)
				assert.Equal(t, serviceAutodiscoveryName, e.HandlerName)
			}
		})
	}
}

func TestServiceHandler_Handle_NilCR(t *testing.T) {
	h := newServiceHandler()
	status, err := h.Handle(context.Background(), instrumentation.EventCreate, nil)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionUnknown, status.Status)
	assert.Equal(t, "MissingResource", status.Reason)
}

func TestServiceHandler_Handle_CreateStoresTemplates(t *testing.T) {
	h := newServiceHandler()

	cr := newServiceCR("redis-check", "default", "redis-svc", []datadoghq.DatadogInstrumentationCheckConfig{
		{
			Integration: "redisdb",
			Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"host": "%%host%%"})},
		},
	})

	status, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionTrue, status.Status)
	assert.Equal(t, "Configured", status.Reason)
	assert.Contains(t, status.Message, "1 check(s) configured")

	// Verify templates are stored for the target service
	templates := h.templateStore.templatesForService("default", "redis-svc")
	require.Len(t, templates, 1)
	assert.Equal(t, "redisdb", templates[0].Name)
	assert.Equal(t, "datadoginstrumentation:default/redis-check", templates[0].Source)
}

func TestServiceHandler_Handle_Delete(t *testing.T) {
	h := newServiceHandler()

	cr := newServiceCR("redis-check", "default", "redis-svc", []datadoghq.DatadogInstrumentationCheckConfig{
		{
			Integration: "redisdb",
			Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"host": "%%host%%"})},
		},
	})

	// Create first
	_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	assert.NotEmpty(t, h.templateStore.templatesForService("default", "redis-svc"))

	// Then delete
	status, err := h.Handle(context.Background(), instrumentation.EventDelete, cr)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionTrue, status.Status)
	assert.Equal(t, "Deleted", status.Reason)
	assert.Empty(t, h.templateStore.templatesForService("default", "redis-svc"))
}

func TestServiceHandler_Handle_UpdateReplaceTemplates(t *testing.T) {
	h := newServiceHandler()

	cr := newServiceCR("redis-check", "default", "redis-svc", []datadoghq.DatadogInstrumentationCheckConfig{
		{
			Integration: "redisdb",
			Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"host": "%%host%%"})},
		},
	})

	// Create with 1 check
	_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	assert.Len(t, h.templateStore.templatesForService("default", "redis-svc"), 1)

	// Update with 2 checks
	cr.Spec.Config.Checks = append(cr.Spec.Config.Checks, datadoghq.DatadogInstrumentationCheckConfig{
		Integration: "http_check",
		Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"url": "http://localhost"})},
	})
	status, err := h.Handle(context.Background(), instrumentation.EventUpdate, cr)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionTrue, status.Status)
	assert.Contains(t, status.Message, "2 check(s) configured")
	assert.Len(t, h.templateStore.templatesForService("default", "redis-svc"), 2)
}

func TestServiceHandler_Handle_MultipleCRs(t *testing.T) {
	h := newServiceHandler()

	cr1 := newServiceCR("redis", "ns1", "svc-1", []datadoghq.DatadogInstrumentationCheckConfig{
		{Integration: "redisdb", Instances: []runtime.RawExtension{rawJSON(t, map[string]string{"host": "redis"})}},
	})
	cr2 := newServiceCR("nginx", "ns2", "svc-2", []datadoghq.DatadogInstrumentationCheckConfig{
		{Integration: "nginx", Instances: []runtime.RawExtension{rawJSON(t, map[string]string{"url": "http://nginx"})}},
	})

	_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr1)
	require.NoError(t, err)
	_, err = h.Handle(context.Background(), instrumentation.EventCreate, cr2)
	require.NoError(t, err)

	assert.Len(t, h.templateStore.templatesForService("ns1", "svc-1"), 1)
	assert.Len(t, h.templateStore.templatesForService("ns2", "svc-2"), 1)

	// Verify tracked services
	tracked := h.templateStore.trackServices()
	assert.Contains(t, tracked, "ns1/svc-1")
	assert.Contains(t, tracked, "ns2/svc-2")

	// Delete first CR; second should remain
	_, err = h.Handle(context.Background(), instrumentation.EventDelete, cr1)
	require.NoError(t, err)
	assert.Empty(t, h.templateStore.templatesForService("ns1", "svc-1"))
	assert.Len(t, h.templateStore.templatesForService("ns2", "svc-2"), 1)
}

func TestServiceCheckTemplateStore_OnChange(t *testing.T) {
	store := NewServiceCheckTemplateStore()

	changeCount := 0
	store.SetOnChange(func() {
		changeCount++
	})

	store.setTemplates("default/test", "default", "svc", nil)
	assert.Equal(t, 1, changeCount)

	store.setTemplates("default/test", "default", "svc", []integration.Config{{Name: "check"}})
	assert.Equal(t, 2, changeCount)

	store.deleteTemplates("default/test")
	assert.Equal(t, 3, changeCount)
}
