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
	"k8s.io/apimachinery/pkg/types"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

// templatesForService returns all check templates targeting a given service.
func (s *ServiceCheckTemplateStore) templatesForService(namespace, name string) []integration.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []integration.Config
	for _, entry := range s.entries {
		if entry.serviceNamespace == namespace && entry.serviceName == name {
			out = append(out, entry.templates...)
		}
	}
	return out
}

func newHandler() (*ChecksHandler, *CheckStore, *ServiceCheckTemplateStore) {
	cs := NewCheckStore()
	ts := NewServiceCheckTemplateStore()
	h := &ChecksHandler{
		checkStore:           cs,
		templateStore:        ts,
		serviceTargetEnabled: true,
	}
	return h, cs, ts
}

// configsForCR returns the stored configs for the given CR, abstracting the
// difference between the check store (workload) and template store (service).
func configsForCR(cr *datadoghq.DatadogInstrumentation, cs *CheckStore, ts *ServiceCheckTemplateStore) []integration.Config {
	if isService(cr) {
		return ts.templatesForService(cr.Namespace, cr.Spec.TargetRef.Name)
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
				{Integration: "redisdb", Instances: []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})}},
			})

			_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
			require.NoError(t, err)
			assert.Len(t, configsForCR(cr, cs, ts), 1)

			cr.Spec.Config.Checks = append(cr.Spec.Config.Checks, datadoghq.DatadogInstrumentationCheckConfig{
				Integration: "nginx",
				Instances:   []runtime.RawExtension{rawJSON(t, map[string]string{"url": "http://localhost"})},
			})
			status, err := h.Handle(context.Background(), instrumentation.EventUpdate, cr)
			require.NoError(t, err)
			assert.Equal(t, metav1.ConditionTrue, status.Status)
			assert.Contains(t, status.Message, "2 check(s) configured")
			assert.Len(t, configsForCR(cr, cs, ts), 2)
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

func TestServiceCheckTemplateStore_NotifyOnChange(t *testing.T) {
	store := NewServiceCheckTemplateStore()

	var notified []string
	store.NotifyOnChange(func(namespace, name string) {
		notified = append(notified, namespace+"/"+name)
	})
	// A second subscriber must also be invoked.
	secondCount := 0
	store.NotifyOnChange(func(string, string) {
		secondCount++
	})

	cr := newCR("test", "default", "Service", "svc", nil)

	// Create: service is notified.
	store.writeTemplates("default/test", cr, []integration.Config{{Name: "check"}})
	require.Equal(t, []string{"default/svc"}, notified)
	assert.Equal(t, 1, secondCount)

	// Config-only update on the same service: single notification (deduplicated).
	store.writeTemplates("default/test", cr, []integration.Config{{Name: "check2"}})
	require.Equal(t, []string{"default/svc", "default/svc"}, notified)
	assert.Equal(t, 2, secondCount)

	// Delete: service is notified.
	store.deleteTemplates("default/test")
	require.Equal(t, []string{"default/svc", "default/svc", "default/svc"}, notified)
	assert.Equal(t, 3, secondCount)

	// No-op write (no prior entry, no configs) notifies nothing.
	store.writeTemplates("default/missing", cr, nil)
	assert.Len(t, notified, 3)
}

func TestTranslateCheck(t *testing.T) {
	tests := []struct {
		name             string
		check            datadoghq.DatadogInstrumentationCheckConfig
		expectedInit     string
		expectedInstLen  int
		expectedADIDs    []string
		instanceContains []string
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{tt.check})
			h, cs, _ := newHandler()
			_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
			require.NoError(t, err)

			configs, _ := cs.ListConfigs()
			require.Len(t, configs, 1)
			assert.Equal(t, tt.expectedInit, string(configs[0].InitConfig))
			require.Len(t, configs[0].Instances, tt.expectedInstLen)
			require.ElementsMatch(t, tt.expectedADIDs, configs[0].ADIdentifiers)

			for i, substr := range tt.instanceContains {
				assert.Contains(t, string(configs[0].Instances[i]), substr)
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
			rules := rootOwnerCELFilter(ref, tt.namespace)
			require.Len(t, rules.Containers, 1)
			for _, substr := range tt.contains {
				assert.Contains(t, rules.Containers[0], substr)
			}
			assert.NotContains(t, rules.Containers[0], `container.name ==`)
		})
	}
}

// TestCheckStoreIncrementalHash verifies the incremental configHash maintained by
// writeConfigs/deleteConfigs.
func TestCheckStoreIncrementalHash(t *testing.T) {
	cfg := []integration.Config{{Name: "check"}}
	write := func(cs *CheckStore, key, uid string, gen int64) {
		cr := newCR(key, "ns", "Deployment", "app", nil)
		cr.UID = types.UID(uid)
		cr.Generation = gen
		cs.writeConfigs(key, cr, cfg)
	}

	t.Run("empty store hashes to zero", func(t *testing.T) {
		require.Equal(t, uint64(0), NewCheckStore().Hash())
	})

	t.Run("add then delete restores empty hash", func(t *testing.T) {
		cs := NewCheckStore()
		write(cs, "a", "uid-a", 1)
		require.NotEqual(t, uint64(0), cs.Hash())
		cs.deleteConfigs("a")
		require.Equal(t, uint64(0), cs.Hash())
	})

	t.Run("update changes hash and revert restores it", func(t *testing.T) {
		cs := NewCheckStore()
		write(cs, "a", "uid-a", 1)
		write(cs, "b", "uid-b", 1)
		full := cs.Hash()
		write(cs, "b", "uid-b", 2) // generation bump
		require.NotEqual(t, full, cs.Hash())
		write(cs, "b", "uid-b", 1) // revert
		require.Equal(t, full, cs.Hash())
	})

	t.Run("hash is independent of apply order", func(t *testing.T) {
		cs := NewCheckStore()
		write(cs, "a", "uid-a", 1)
		write(cs, "b", "uid-b", 1)

		other := NewCheckStore()
		write(other, "b", "uid-b", 1)
		write(other, "a", "uid-a", 1)

		require.Equal(t, cs.Hash(), other.Hash())
	})

	t.Run("hash is different when same cr has new uid", func(t *testing.T) {
		cs := NewCheckStore()
		write(cs, "a", "uid-a", 1)
		prevHash := cs.Hash()
		write(cs, "a", "uid-b", 1)
		require.NotEqual(t, prevHash, cs.Hash())
	})
}
