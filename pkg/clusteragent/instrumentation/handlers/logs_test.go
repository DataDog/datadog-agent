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

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

func newLogsHandler() (*LogsHandler, *CheckStore) {
	cs := NewCheckStore()
	h := &LogsHandler{
		checkStore: cs,
	}
	return h, cs
}

func newLogsCR(name, namespace string, targetKind, targetName string, logs []datadoghq.DatadogInstrumentationLogConfig) *datadoghq.DatadogInstrumentation {
	cr := newCR(name, namespace, targetKind, targetName, nil)
	cr.Spec.Config.Logs = logs
	return cr
}

func TestLogsHasSection(t *testing.T) {
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
			name:     "no logs",
			cr:       newLogsCR("test", "default", "Deployment", "app", nil),
			expected: false,
		},
		{
			name: "with logs",
			cr: newLogsCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationLogConfig{
				{ContainerName: "app"},
			}),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newLogsHandler()
			assert.Equal(t, tt.expected, h.HasSection(tt.cr))
		})
	}
}

func TestLogsSupportsTarget(t *testing.T) {
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
	}

	h, _ := newLogsHandler()
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			ref := autoscalingv2.CrossVersionObjectReference{Kind: tt.kind, Name: "test"}
			assert.Equal(t, tt.expected, h.SupportsTarget(ref))
		})
	}
}

func TestLogsValidate(t *testing.T) {
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
			name: "valid log with container name",
			cr: newLogsCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationLogConfig{
				{ContainerName: "app", DatadogInstrumentationLogFields: datadoghq.DatadogInstrumentationLogFields{Source: "nginx"}},
			}),
			expectErrCount: 0,
		},
		{
			name: "missing container name",
			cr: newLogsCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationLogConfig{
				{DatadogInstrumentationLogFields: datadoghq.DatadogInstrumentationLogFields{Source: "nginx"}},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.logs[0].containerName",
		},
		{
			name: "whitespace container name",
			cr: newLogsCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationLogConfig{
				{ContainerName: "   "},
			}),
			expectErrCount: 1,
			expectField:    "spec.config.logs[0].containerName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newLogsHandler()
			errs := h.Validate(tt.cr)
			assert.Len(t, errs, tt.expectErrCount)
			if tt.expectField != "" && len(errs) > 0 {
				assert.Equal(t, tt.expectField, errs[0].Field)
			}
		})
	}
}

func TestLogsHandle_CreateAndDelete(t *testing.T) {
	port := int32(10514)
	tests := []struct {
		name       string
		targetKind string
		targetName string
	}{
		{"deployment", "Deployment", "app"},
		{"job", "Job", "worker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cs := newLogsHandler()
			cr := newLogsCR("test", "default", tt.targetKind, tt.targetName, []datadoghq.DatadogInstrumentationLogConfig{
				{
					ContainerName: "app",
					DatadogInstrumentationLogFields: datadoghq.DatadogInstrumentationLogFields{
						Type:    "tcp",
						Port:    &port,
						Service: "web",
						Source:  "nginx",
					},
				},
			})

			status, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
			require.NoError(t, err)
			assert.Equal(t, metav1.ConditionTrue, status.Status)
			assert.Equal(t, "Configured", status.Reason)
			assert.Contains(t, status.Message, "1 log config(s) configured")

			configs, _ := cs.ListConfigs()
			require.Len(t, configs, 1)
			assert.Equal(t, logsCheckName, configs[0].Name)
			assert.Equal(t, []string{adtypes.KubeContainerNameIdentifier("app")}, configs[0].ADIdentifiers)
			assert.Equal(t, "datadoginstrumentation:default/test", configs[0].Source)
			assert.Contains(t, string(configs[0].LogsConfig), `"type":"tcp"`)
			assert.Contains(t, string(configs[0].LogsConfig), `"service":"web"`)
			assert.NotContains(t, string(configs[0].LogsConfig), "containerName")
			require.Len(t, configs[0].CELSelector.Containers, 1)
			assert.Contains(t, configs[0].CELSelector.Containers[0], `container.pod.rootowner.name == "`+tt.targetName+`"`)

			status, err = h.Handle(context.Background(), instrumentation.EventDelete, cr)
			require.NoError(t, err)
			assert.Equal(t, metav1.ConditionTrue, status.Status)
			assert.Equal(t, "Deleted", status.Reason)
			configs, _ = cs.ListConfigs()
			assert.Empty(t, configs)
		})
	}
}

func TestLogsHandle_Update(t *testing.T) {
	h, cs := newLogsHandler()
	cr := newLogsCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationLogConfig{
		{
			ContainerName: "app",
			DatadogInstrumentationLogFields: datadoghq.DatadogInstrumentationLogFields{
				Source: "nginx",
			},
		},
	})

	_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	configs, _ := cs.ListConfigs()
	require.Len(t, configs, 1)

	cr.Spec.Config.Logs = append(cr.Spec.Config.Logs, datadoghq.DatadogInstrumentationLogConfig{
		ContainerName: "sidecar",
		DatadogInstrumentationLogFields: datadoghq.DatadogInstrumentationLogFields{
			Source: "envoy",
		},
	})
	status, err := h.Handle(context.Background(), instrumentation.EventUpdate, cr)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionTrue, status.Status)
	assert.Contains(t, status.Message, "2 log config(s) configured")

	configs, _ = cs.ListConfigs()
	require.Len(t, configs, 2)
	assert.ElementsMatch(t, []string{
		adtypes.KubeContainerNameIdentifier("app"),
		adtypes.KubeContainerNameIdentifier("sidecar"),
	}, []string{configs[0].ADIdentifiers[0], configs[1].ADIdentifiers[0]})
}

func TestLogsHandle_MultipleCRs(t *testing.T) {
	h, cs := newLogsHandler()
	cr1 := newLogsCR("nginx-logs", "ns1", "Deployment", "nginx", []datadoghq.DatadogInstrumentationLogConfig{
		{
			ContainerName: "nginx",
			DatadogInstrumentationLogFields: datadoghq.DatadogInstrumentationLogFields{
				Source: "nginx",
			},
		},
	})
	cr2 := newLogsCR("envoy-logs", "ns2", "Deployment", "envoy", []datadoghq.DatadogInstrumentationLogConfig{
		{
			ContainerName: "envoy",
			DatadogInstrumentationLogFields: datadoghq.DatadogInstrumentationLogFields{
				Source: "envoy",
			},
		},
	})

	_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr1)
	require.NoError(t, err)
	_, err = h.Handle(context.Background(), instrumentation.EventCreate, cr2)
	require.NoError(t, err)

	configs, _ := cs.ListConfigs()
	require.Len(t, configs, 2)

	_, err = h.Handle(context.Background(), instrumentation.EventDelete, cr1)
	require.NoError(t, err)

	configs, _ = cs.ListConfigs()
	require.Len(t, configs, 1)
	assert.Equal(t, "datadoginstrumentation:ns2/envoy-logs", configs[0].Source)
	assert.Equal(t, []string{adtypes.KubeContainerNameIdentifier("envoy")}, configs[0].ADIdentifiers)
}

func TestLogsHandler_NilCR(t *testing.T) {
	h, _ := newLogsHandler()
	status, err := h.Handle(context.Background(), instrumentation.EventCreate, nil)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionUnknown, status.Status)
	assert.Equal(t, "MissingResource", status.Reason)
}

func TestLogsHandlerSharesCheckStoreWithoutOverwritingChecks(t *testing.T) {
	cs := NewCheckStore()
	ts := NewServiceCheckTemplateStore()
	checksHandler := &ChecksHandler{
		checkStore:           cs,
		templateStore:        ts,
		serviceTargetEnabled: true,
	}
	logsHandler := &LogsHandler{checkStore: cs}

	cr := newCR("test", "default", "Deployment", "app", []datadoghq.DatadogInstrumentationCheckConfig{
		{
			Integration:   "redisdb",
			ContainerName: "app",
			Instances:     []runtime.RawExtension{rawJSON(t, map[string]string{"host": "localhost"})},
		},
	})
	cr.Spec.Config.Logs = []datadoghq.DatadogInstrumentationLogConfig{
		{
			ContainerName: "app",
			DatadogInstrumentationLogFields: datadoghq.DatadogInstrumentationLogFields{
				Source: "redis",
			},
		},
	}

	_, err := checksHandler.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	_, err = logsHandler.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)

	configs, _ := cs.ListConfigs()
	require.Len(t, configs, 2)

	byName := map[string]integration.Config{}
	for _, cfg := range configs {
		byName[cfg.Name] = cfg
	}
	assert.Contains(t, byName, "redisdb")
	assert.Contains(t, byName, logsCheckName)
	assert.NotNil(t, byName[logsCheckName].LogsConfig)
	assert.Len(t, byName["redisdb"].Instances, 1)

	_, err = logsHandler.Handle(context.Background(), instrumentation.EventDelete, cr)
	require.NoError(t, err)

	configs, _ = cs.ListConfigs()
	require.Len(t, configs, 1)
	assert.Equal(t, "redisdb", configs[0].Name)
}
