// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"encoding/json"
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

func newHandler() *AutodiscoveryHandler {
	return NewAutodiscoveryHandler(&Deps{
		CheckStore: NewCheckStore(),
	})
}

func newCR(name, namespace string, targetKind, targetName string, checks []datadoghq.DatadogInstrumentationCheckConfig) *datadoghq.DatadogInstrumentation {
	return &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: datadoghq.DatadogInstrumentationSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: targetKind,
				Name: targetName,
			},
			Config: datadoghq.DatadogInstrumentationConfig{
				Checks: checks,
			},
		},
	}
}

func rawJSON(t *testing.T, v interface{}) runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return runtime.RawExtension{Raw: b}
}

func TestName(t *testing.T) {
	h := newHandler()
	assert.Equal(t, "autodiscovery", h.Name())
}

func TestHasSection(t *testing.T) {
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
			cr:       newCR("test", "default", "Deployment", "app", nil),
			expected: false,
		},
		{
			name: "with checks",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{Integration: "redisdb"},
			}),
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandler()
			assert.Equal(t, tt.expected, h.HasSection(tt.cr))
		})
	}
}

func TestSupportsTarget(t *testing.T) {
	tests := []struct {
		kind     string
		expected bool
	}{
		{"Deployment", true},
		{"DaemonSet", true},
		{"StatefulSet", true},
		{"CronJob", true},
		{"Job", true},
		{"Service", false},
		{"ReplicaSet", false},
		{"Pod", false},
		{"", false},
	}
	h := newHandler()
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			ref := autoscalingv2.CrossVersionObjectReference{Kind: tt.kind, Name: "test"}
			assert.Equal(t, tt.expected, h.SupportsTarget(ref))
		})
	}
}

func TestValidate(t *testing.T) {
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
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
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
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
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
			name: "whitespace-only integration name",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:    "   ",
					ContainerImage: []string{"redis"},
					Instances:      []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].integration",
		},
		{
			name: "no instances or logs",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:    "redisdb",
					ContainerImage: []string{"redis"},
					Instances:      nil,
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].instances",
		},
		{
			name: "logs only is valid",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:    "custom",
					ContainerImage: []string{"app"},
					Logs:           []datadoghq.DatadogInstrumentationLogConfig{{Type: "tcp"}},
				},
			}),
			expectErrCount: 0,
		},
		{
			name: "no container image",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration: "redisdb",
					Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].containerImage",
		},
		{
			name: "all validations fail",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{Integration: "", Instances: nil},
			}),
			expectErrCount: 3,
		},
		{
			name: "multiple checks with mixed errors",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:    "redisdb",
					ContainerImage: []string{"redis"},
					Instances:      []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
				{
					Integration:    "",
					ContainerImage: []string{"app"},
					Instances:      []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[1].integration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandler()
			errs := h.Validate(tt.cr)
			assert.Len(t, errs, tt.expectErrCount)
			if tt.expectField != "" && len(errs) > 0 {
				assert.Equal(t, tt.expectField, errs[0].Field)
			}
			for _, e := range errs {
				assert.Equal(t, checksReadyConditionType, e.Type)
				assert.Equal(t, "autodiscovery", e.HandlerName)
			}
		})
	}
}

func TestHandle_NilCR(t *testing.T) {
	h := newHandler()
	status, err := h.Handle(context.Background(), instrumentation.EventCreate, nil)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionUnknown, status.Status)
	assert.Equal(t, "MissingResource", status.Reason)
}

func TestHandle_Delete(t *testing.T) {
	h := newHandler()
	cr := newCR("test", "default", "Deployment", "my-app", []datadoghq.DatadogInstrumentationCheckConfig{
		{
			Integration: "redisdb",
			Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
		},
	})

	// First create checkStore
	_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	assert.Len(t, h.checkStore.ListConfigs(), 1)

	// Then delete
	status, err := h.Handle(context.Background(), instrumentation.EventDelete, cr)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionTrue, status.Status)
	assert.Equal(t, "Deleted", status.Reason)
	assert.Empty(t, h.checkStore.ListConfigs())
}

func TestHandle_CreateAndUpdate(t *testing.T) {
	h := newHandler()
	cr := newCR("test", "default", "Deployment", "my-app", []datadoghq.DatadogInstrumentationCheckConfig{
		{
			Integration: "redisdb",
			Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
		},
	})

	// Create
	status, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionTrue, status.Status)
	assert.Equal(t, "Configured", status.Reason)
	assert.Contains(t, status.Message, "1 check(s) configured")

	configs := h.checkStore.ListConfigs()
	require.Len(t, configs, 1)
	assert.Equal(t, "redisdb", configs[0].Name)
	assert.Equal(t, "datadoginstrumentation:default/test", configs[0].Source)

	// Update with two checks
	cr.Spec.Config.Checks = append(cr.Spec.Config.Checks, datadoghq.DatadogInstrumentationCheckConfig{
		Integration: "nginx",
		Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"url": "http://localhost"})},
	})
	status, err = h.Handle(context.Background(), instrumentation.EventUpdate, cr)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionTrue, status.Status)
	assert.Contains(t, status.Message, "2 check(s) configured")
	assert.Len(t, h.checkStore.ListConfigs(), 2)
}

func TestHandle_MultipleCRs(t *testing.T) {
	h := newHandler()
	cr1 := newCR("redis-check", "ns1", "Deployment", "redis", []datadoghq.DatadogInstrumentationCheckConfig{
		{Integration: "redisdb", Instances: []runtime.RawExtension{rawJSON(t, map[string]string{"host": "redis"})}},
	})
	cr2 := newCR("nginx-check", "ns2", "DaemonSet", "nginx", []datadoghq.DatadogInstrumentationCheckConfig{
		{Integration: "nginx", Instances: []runtime.RawExtension{rawJSON(t, map[string]string{"url": "http://nginx"})}},
	})

	_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr1)
	require.NoError(t, err)
	_, err = h.Handle(context.Background(), instrumentation.EventCreate, cr2)
	require.NoError(t, err)
	assert.Len(t, h.checkStore.ListConfigs(), 2)

	// Delete first CR; second should remain
	_, err = h.Handle(context.Background(), instrumentation.EventDelete, cr1)
	require.NoError(t, err)
	configs := h.checkStore.ListConfigs()
	require.Len(t, configs, 1)
	assert.Equal(t, "nginx", configs[0].Name)
}

func TestTranslateCheck(t *testing.T) {
	port := int32(10514)
	tests := []struct {
		name             string
		check            datadoghq.DatadogInstrumentationCheckConfig
		expectedInit     string
		expectedInstLen  int
		expectedADIDs    []string
		instanceContains []string
		logsNil          bool
		logsContains     string
	}{
		{
			name: "empty init config defaults to {}",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration:    "http_check",
				ContainerImage: []string{"container-image"},
				Instances:      []runtime.RawExtension{{Raw: []byte(`{"url":"http://localhost"}`)}},
			},
			expectedInit:    "{}",
			expectedInstLen: 1,
			expectedADIDs:   []string{"container-image"},
			logsNil:         true,
		},
		{
			name: "provided init config is preserved",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration:    "http_check",
				ContainerImage: []string{"container-image"},
				InitConfig:     runtime.RawExtension{Raw: []byte(`{"service":"myservice"}`)},
				Instances:      []runtime.RawExtension{{Raw: []byte(`{"url":"http://localhost"}`)}},
			},
			expectedInit:    `{"service":"myservice"}`,
			expectedInstLen: 1,
			expectedADIDs:   []string{"container-image"},
			logsNil:         true,
		},
		{
			name: "multiple instances are translated",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration:    "http_check",
				ContainerImage: []string{"container-image", "other-container"},
				Instances: []runtime.RawExtension{
					{Raw: []byte(`{"url":"http://host1"}`)},
					{Raw: []byte(`{"url":"http://host2"}`)},
				},
			},
			expectedInit:     "{}",
			expectedInstLen:  2,
			expectedADIDs:    []string{"container-image", "other-container"},
			instanceContains: []string{"host1", "host2"},
			logsNil:          true,
		},
		{
			name: "logs config is translated",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration: "custom",
				Instances:   []runtime.RawExtension{{Raw: []byte(`{"key":"val"}`)}},
				Logs:        []datadoghq.DatadogInstrumentationLogConfig{{Type: "tcp", Port: &port}},
			},
			expectedInit:    "{}",
			expectedInstLen: 1,
			logsContains:    `"type":"tcp"`,
		},
		{
			name: "no logs returns nil logs config",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration: "redisdb",
				Instances:   []runtime.RawExtension{{Raw: []byte(`{"host":"localhost"}`)}},
			},
			expectedInit:    "{}",
			expectedInstLen: 1,
			logsNil:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{tt.check})
			h := newHandler()
			_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
			require.NoError(t, err)

			configs := h.checkStore.ListConfigs()
			require.Len(t, configs, 1)
			assert.Equal(t, tt.expectedInit, string(configs[0].InitConfig))
			require.Len(t, configs[0].Instances, tt.expectedInstLen)

			if tt.expectedADIDs != nil {
				require.ElementsMatch(t, tt.expectedADIDs, configs[0].ADIdentifiers)
			}

			for i, substr := range tt.instanceContains {
				assert.Contains(t, string(configs[0].Instances[i]), substr)
			}
			if tt.logsNil {
				assert.Nil(t, configs[0].LogsConfig)
			} else {
				assert.Contains(t, string(configs[0].LogsConfig), tt.logsContains)
			}
		})
	}
}

func TestBuildCELSelector(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		target    string
		namespace string
		contains  []string
	}{
		{
			name:      "basic selector without images",
			kind:      "Deployment",
			target:    "my-app",
			namespace: "default",
			contains: []string{
				`container.pod.rootowner.kind == "Deployment"`,
				`container.pod.rootowner.name == "my-app"`,
				`container.pod.namespace == "default"`,
			},
		},
		{
			name:      "selector with single image",
			kind:      "StatefulSet",
			target:    "redis",
			namespace: "data",
			contains: []string{
				`container.pod.rootowner.kind == "StatefulSet"`,
			},
		},
		{
			name:      "selector with multiple images",
			kind:      "Deployment",
			target:    "multi",
			namespace: "default",
			contains: []string{
				`container.pod.rootowner.kind == "Deployment"`,
				`container.pod.rootowner.name == "multi"`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := autoscalingv2.CrossVersionObjectReference{Kind: tt.kind, Name: tt.target}
			rules := buildCELSelector(ref, tt.namespace)
			require.Len(t, rules.Containers, 1)
			for _, substr := range tt.contains {
				assert.Contains(t, rules.Containers[0], substr)
			}
		})
	}
}
