// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package instrumentation

import (
	"context"
	"sync"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newTestCR(name, namespace string, generation int64, checks []datadoghq.DatadogInstrumentationCheckConfig) *datadoghq.DatadogInstrumentation {
	return &datadoghq.DatadogInstrumentation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogInstrumentation",
			APIVersion: datadoghq.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: generation,
		},
		Spec: datadoghq.DatadogInstrumentationSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: "my-app",
			},
			Config: datadoghq.DatadogInstrumentationConfig{
				Checks: checks,
			},
		},
	}
}

func defaultChecks() []datadoghq.DatadogInstrumentationCheckConfig {
	return []datadoghq.DatadogInstrumentationCheckConfig{
		{Integration: "redisdb"},
	}
}

func fakeScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: datadoghq.GroupVersion.Group, Version: datadoghq.GroupVersion.Version, Kind: "DatadogInstrumentation"},
		&datadoghq.DatadogInstrumentation{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: datadoghq.GroupVersion.Group, Version: datadoghq.GroupVersion.Version, Kind: "DatadogInstrumentationList"},
		&datadoghq.DatadogInstrumentationList{},
	)
	return scheme
}

// mockHandler is a test double for the Handler interface that records calls.
type handleCall struct {
	eventType EventType
	cr        *datadoghq.DatadogInstrumentation
}

type mockHandler struct {
	name           string
	hasSectionFunc func(*datadoghq.DatadogInstrumentation) bool
	conditionType  string
	handleStatus   HandlerStatus
	handleErr      error

	mu    sync.Mutex
	calls []handleCall
}

func (m *mockHandler) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

func (m *mockHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return m.hasSectionFunc(cr)
}

func (m *mockHandler) SupportsTarget(_ autoscalingv2.CrossVersionObjectReference) bool {
	return true
}

func (m *mockHandler) Handle(_ context.Context, eventType EventType, cr *datadoghq.DatadogInstrumentation) (HandlerStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, handleCall{eventType: eventType, cr: cr})
	return m.handleStatus, m.handleErr
}

func (m *mockHandler) Validate(_ *datadoghq.DatadogInstrumentation) []ValidationError {
	return nil
}

func (m *mockHandler) ConditionType() string {
	return m.conditionType
}

func (m *mockHandler) getCalls() []handleCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]handleCall, len(m.calls))
	copy(out, m.calls)
	return out
}
