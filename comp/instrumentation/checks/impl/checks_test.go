// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package checksimpl

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

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/instrumentation/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

func newHandler() (*checksHandler, *common.CheckStore, *common.ServiceCheckTemplateStore) {
	cs := common.NewCheckStore()
	ts := common.NewServiceCheckTemplateStore()
	h := &checksHandler{
		checkStore:           cs,
		templateStore:        ts,
		serviceTargetEnabled: true,
	}
	return h, cs, ts
}

// configsForCR returns the stored configs for the given CR, abstracting the
// difference between the check store (workload) and template store (service).
func configsForCR(cr *datadoghq.DatadogInstrumentation, cs *common.CheckStore, ts *common.ServiceCheckTemplateStore) []integration.Config {
	if isService(cr) {
		return ts.AllTemplatesByService()[cr.Namespace+"/"+cr.Spec.TargetRef.Name]
	}
	configs, _ := cs.ListConfigs()
	return configs
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
			name: "workload with checks",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{Integration: "redisdb"},
			}),
			expected: true,
		},
		{
			name: "service with checks",
			cr: newCR("test", "default", "Service", "my-svc", []datadoghq.DatadogInstrumentationCheckConfig{
				{Integration: "redisdb"},
			}),
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _ := newHandler()
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
		{"Service", true},
		{"Rollout", true},
		{"StrimziPodSet", true},
		{"ReplicaSet", false},
		{"Pod", false},
		{"", false},
	}
	h, _, _ := newHandler()
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			ref := autoscalingv2.CrossVersionObjectReference{Kind: tt.kind, Name: "test"}
			assert.Equal(t, tt.expected, h.SupportsTarget(ref))
		})
	}

	t.Run("Service without EndpointSlices", func(t *testing.T) {
		h.SetServiceTargetEnabled(false)
		assert.False(t, h.SupportsTarget(autoscalingv2.CrossVersionObjectReference{Kind: "Service"}))
		assert.True(t, h.SupportsTarget(autoscalingv2.CrossVersionObjectReference{Kind: "Deployment"}))
	})
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
			name: "valid check with container name",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:   "redisdb",
					ContainerName: "redis",
					Instances:     []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 0,
		},
		{
			name: "empty integration name",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:   "",
					ContainerName: "redis",
					Instances:     []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].integration",
		},
		{
			name: "whitespace-only integration name",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:   "   ",
					ContainerName: "redis",
					Instances:     []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].integration",
		},
		{
			name: "no instances",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:   "redisdb",
					ContainerName: "redis",
					Instances:     nil,
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].instances",
		},
		{
			name: "no container name for workload",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration: "redisdb",
					Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[0].containerName",
		},
		{
			name: "all validations fail for workload",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{Integration: "", Instances: nil},
			}),
			expectErrCount: 3,
		},
		{
			name: "multiple checks with mixed errors",
			cr: newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:   "redisdb",
					ContainerName: "redis",
					Instances:     []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
				{
					Integration:   "",
					ContainerName: "app",
					Instances:     []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
				},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.checks[1].integration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _ := newHandler()
			errs := h.Validate(tt.cr)
			assert.Len(t, errs, tt.expectErrCount)
			if tt.expectField != "" && len(errs) > 0 {
				assert.Equal(t, tt.expectField, errs[0].Field)
			}
		})
	}
}

func TestHandle_NilCR(t *testing.T) {
	h, _, _ := newHandler()
	status, err := h.Handle(context.Background(), instrumentation.EventCreate, nil)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionUnknown, status.Status)
	assert.Equal(t, "MissingResource", status.Reason)
}

func TestHandle_CreateAndDelete(t *testing.T) {
	tests := []struct {
		name       string
		targetKind string
		targetName string
	}{
		{"workload", "Deployment", "my-app"},
		{"service", "Service", "my-svc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cs, ts := newHandler()
			cr := newCR("test", "default", tt.targetKind, tt.targetName, []datadoghq.DatadogInstrumentationCheckConfig{
				{Integration: "redisdb", Instances: []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})}},
			})

			// Create
			status, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
			require.NoError(t, err)
			assert.Equal(t, metav1.ConditionTrue, status.Status)
			assert.Equal(t, "Configured", status.Reason)
			assert.Contains(t, status.Message, "1 check(s) configured")

			configs := configsForCR(cr, cs, ts)
			require.Len(t, configs, 1)
			assert.Equal(t, "redisdb", configs[0].Name)
			assert.Equal(t, "datadoginstrumentation:default/test", configs[0].Source)

			// Delete
			status, err = h.Handle(context.Background(), instrumentation.EventDelete, cr)
			require.NoError(t, err)
			assert.Equal(t, metav1.ConditionTrue, status.Status)
			assert.Equal(t, "Deleted", status.Reason)
			assert.Empty(t, configsForCR(cr, cs, ts))
		})
	}
}

func TestHandle_Update(t *testing.T) {
	tests := []struct {
		name       string
		targetKind string
		targetName string
	}{
		{"workload", "Deployment", "my-app"},
		{"service", "Service", "my-svc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cs, ts := newHandler()
			cr := newCR("test", "default", tt.targetKind, tt.targetName, []datadoghq.DatadogInstrumentationCheckConfig{
				{
					Integration:             "redisdb",
					Instances:               []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
					IgnoreAutodiscoveryTags: true,
					CheckTagCardinality:     "high",
				},
			})

			_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
			require.NoError(t, err)
			configs := configsForCR(cr, cs, ts)
			require.Len(t, configs, 1)
			assert.True(t, configs[0].IgnoreAutodiscoveryTags)
			assert.Equal(t, "high", configs[0].CheckTagCardinality)

			// Removing options restores the inherited Agent settings.
			cr.Spec.Config.Checks[0].IgnoreAutodiscoveryTags = false
			cr.Spec.Config.Checks[0].CheckTagCardinality = ""
			cr.Generation++

			cr.Spec.Config.Checks = append(cr.Spec.Config.Checks, datadoghq.DatadogInstrumentationCheckConfig{
				Integration: "nginx",
				Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"url": "http://localhost"})},
			})
			status, err := h.Handle(context.Background(), instrumentation.EventUpdate, cr)
			require.NoError(t, err)
			assert.Equal(t, metav1.ConditionTrue, status.Status)
			assert.Contains(t, status.Message, "2 check(s) configured")
			configs = configsForCR(cr, cs, ts)
			require.Len(t, configs, 2)
			for _, cfg := range configs {
				assert.False(t, cfg.IgnoreAutodiscoveryTags)
				assert.Empty(t, cfg.CheckTagCardinality)
			}
		})
	}
}

func TestHandle_MultipleCRs(t *testing.T) {
	tests := []struct {
		name       string
		targetKind string
	}{
		{"workload", "Deployment"},
		{"service", "Service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cs, ts := newHandler()
			cr1 := newCR("redis-check", "ns1", tt.targetKind, "target-1", []datadoghq.DatadogInstrumentationCheckConfig{
				{Integration: "redisdb", Instances: []runtime.RawExtension{rawJSON(t, map[string]string{"host": "redis"})}},
			})
			cr2 := newCR("nginx-check", "ns2", tt.targetKind, "target-2", []datadoghq.DatadogInstrumentationCheckConfig{
				{Integration: "nginx", Instances: []runtime.RawExtension{rawJSON(t, map[string]string{"url": "http://nginx"})}},
			})

			_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr1)
			require.NoError(t, err)
			_, err = h.Handle(context.Background(), instrumentation.EventCreate, cr2)
			require.NoError(t, err)

			// Delete first CR; second should remain
			_, err = h.Handle(context.Background(), instrumentation.EventDelete, cr1)
			require.NoError(t, err)

			remaining := configsForCR(cr2, cs, ts)
			require.Len(t, remaining, 1)
			assert.Equal(t, "nginx", remaining[0].Name)
		})
	}
}

func TestTranslateCheck(t *testing.T) {
	tests := []struct {
		name                string
		check               datadoghq.DatadogInstrumentationCheckConfig
		expectedInit        string
		expectedInstLen     int
		expectedADIDs       []string
		instanceContains    []string
		expectedIgnoreTags  bool
		expectedCardinality string
	}{
		{
			name: "empty init config defaults to {}",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration:   "http_check",
				ContainerName: "app",
				Instances:     []runtime.RawExtension{{Raw: []byte(`{"url":"http://localhost"}`)}},
			},
			expectedInit:    "{}",
			expectedInstLen: 1,
			expectedADIDs:   []string{adtypes.KubeContainerNameIdentifier("app")},
		},
		{
			name: "provided init config is preserved",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration:   "http_check",
				ContainerName: "app",
				InitConfig:    runtime.RawExtension{Raw: []byte(`{"service":"myservice"}`)},
				Instances:     []runtime.RawExtension{{Raw: []byte(`{"url":"http://localhost"}`)}},
			},
			expectedInit:    `{"service":"myservice"}`,
			expectedInstLen: 1,
			expectedADIDs:   []string{adtypes.KubeContainerNameIdentifier("app")},
		},
		{
			name: "multiple instances are preserved",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration:   "http_check",
				ContainerName: "app",
				Instances: []runtime.RawExtension{
					{Raw: []byte(`{"url":"http://host1"}`)},
					{Raw: []byte(`{"url":"http://host2"}`)},
				},
			},
			expectedInit:     "{}",
			expectedInstLen:  2,
			expectedADIDs:    []string{adtypes.KubeContainerNameIdentifier("app")},
			instanceContains: []string{"host1", "host2"},
		},
		{
			name: "low cardinality",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration:             "http_check",
				ContainerName:           "app",
				Instances:               []runtime.RawExtension{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
				IgnoreAutodiscoveryTags: false,
				CheckTagCardinality:     "low",
			},
			expectedInit:        "{}",
			expectedInstLen:     1,
			expectedADIDs:       []string{adtypes.KubeContainerNameIdentifier("app")},
			expectedIgnoreTags:  false,
			expectedCardinality: "low",
		},
		{
			name: "orchestrator cardinality",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration:             "http_check",
				ContainerName:           "app",
				Instances:               []runtime.RawExtension{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
				IgnoreAutodiscoveryTags: true,
				CheckTagCardinality:     "orchestrator",
			},
			expectedInit:        "{}",
			expectedInstLen:     1,
			expectedADIDs:       []string{adtypes.KubeContainerNameIdentifier("app")},
			expectedIgnoreTags:  true,
			expectedCardinality: "orchestrator",
		},
		{
			name: "high cardinality",
			check: datadoghq.DatadogInstrumentationCheckConfig{
				Integration:             "http_check",
				ContainerName:           "app",
				Instances:               []runtime.RawExtension{{Raw: []byte(`{"url":"http://%%host%%"}`)}},
				IgnoreAutodiscoveryTags: true,
				CheckTagCardinality:     "high",
			},
			expectedInit:        "{}",
			expectedInstLen:     1,
			expectedADIDs:       []string{adtypes.KubeContainerNameIdentifier("app")},
			expectedIgnoreTags:  true,
			expectedCardinality: "high",
		},
	}
	for _, targetKind := range []string{"Deployment", "Service"} {
		t.Run(targetKind, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					cr := newCR("test", "default", targetKind, "app", []datadoghq.DatadogInstrumentationCheckConfig{tt.check})
					h, cs, ts := newHandler()
					status, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
					require.NoError(t, err)
					configs := configsForCR(cr, cs, ts)
					require.Equal(t, metav1.ConditionTrue, status.Status)
					require.Len(t, configs, 1)
					cfg := configs[0]
					assert.Equal(t, tt.expectedInit, string(cfg.InitConfig))
					require.Len(t, cfg.Instances, tt.expectedInstLen)
					if targetKind == "Service" {
						assert.Empty(t, cfg.ADIdentifiers)
					} else {
						require.ElementsMatch(t, tt.expectedADIDs, cfg.ADIdentifiers)
					}
					for i, substr := range tt.instanceContains {
						assert.Contains(t, string(cfg.Instances[i]), substr)
					}
					assert.Equal(t, tt.expectedIgnoreTags, cfg.IgnoreAutodiscoveryTags)
					assert.Equal(t, tt.expectedCardinality, cfg.CheckTagCardinality)
				})
			}
		})
	}
}

func TestRootOwnerCELFilter(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		target    string
		namespace string
		contains  []string
	}{
		{
			name:      "basic selector",
			kind:      "Deployment",
			target:    "my-app",
			namespace: "default",
			contains: []string{
				`container.pod.rootowner.kind == "Deployment"`,
				`container.pod.rootowner.name == "my-app"`,
				`container.pod.namespace == "default"`,
				`container.image.reference != ""`,
			},
		},
		{
			name:      "selector with statefulset target",
			kind:      "StatefulSet",
			target:    "redis",
			namespace: "data",
			contains: []string{
				`container.pod.rootowner.kind == "StatefulSet"`,
				`container.pod.rootowner.name == "redis"`,
			},
		},
		{
			name:      "selector with deployment target",
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
			rules := common.RootOwnerCELFilter(ref, tt.namespace)
			require.Len(t, rules.Containers, 1)
			for _, substr := range tt.contains {
				assert.Contains(t, rules.Containers[0], substr)
			}
			assert.NotContains(t, rules.Containers[0], `container.name ==`)
		})
	}
}
