// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers_test

import (
	"context"
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation/handlers"
	"github.com/DataDog/datadog-agent/pkg/ssi/crstore"
)

func newDDI(crName, crNamespace, targetKind, targetName string, apm *datadoghq.DatadogInstrumentationAPMConfig) *datadoghq.DatadogInstrumentation {
	return &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: crNamespace,
		},
		Spec: datadoghq.DatadogInstrumentationSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: targetKind,
				Name: targetName,
			},
			Config: datadoghq.DatadogInstrumentationConfig{
				APM: apm,
			},
		},
	}
}

func TestAPMHandler_Name(t *testing.T) {
	h := handlers.NewAPMHandler(handlers.Deps{})
	assert.Equal(t, "apm", h.Name())
}

func TestAPMHandler_HasSection(t *testing.T) {
	h := handlers.NewAPMHandler(handlers.Deps{})

	assert.False(t, h.HasSection(nil))
	assert.False(t, h.HasSection(newDDI("a", "default", "Deployment", "web", nil)))
	assert.True(t, h.HasSection(newDDI("a", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{Enabled: true})))
}

func TestAPMHandler_SupportsTarget(t *testing.T) {
	h := handlers.NewAPMHandler(handlers.Deps{})
	for _, kind := range []string{"Deployment", "StatefulSet", "DaemonSet"} {
		assert.Truef(t, h.SupportsTarget(autoscalingv2.CrossVersionObjectReference{Kind: kind}), "kind %s should be supported", kind)
	}
	for _, kind := range []string{"Job", "CronJob", "Service", "Pod", ""} {
		assert.Falsef(t, h.SupportsTarget(autoscalingv2.CrossVersionObjectReference{Kind: kind}), "kind %s should not be supported", kind)
	}
}

func TestAPMHandler_Validate(t *testing.T) {
	h := handlers.NewAPMHandler(handlers.Deps{})

	t.Run("nil_apm_returns_no_errors", func(t *testing.T) {
		errs := h.Validate(newDDI("a", "default", "Deployment", "web", nil))
		assert.Empty(t, errs)
	})

	t.Run("valid_config", func(t *testing.T) {
		cr := newDDI("a", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{
			Enabled:        true,
			TracerVersions: map[string]string{"java": "v1", "python": "v4"},
			TracerConfigs:  []corev1.EnvVar{{Name: "DD_SERVICE", Value: "svc"}},
		})
		assert.Empty(t, h.Validate(cr))
	})

	t.Run("unsupported_target_kind", func(t *testing.T) {
		cr := newDDI("a", "default", "CronJob", "web", &datadoghq.DatadogInstrumentationAPMConfig{Enabled: true})
		errs := h.Validate(cr)
		require.Len(t, errs, 1)
		assert.Equal(t, "UnsupportedTarget", errs[0].Reason)
		assert.Equal(t, "spec.targetRef.kind", errs[0].Field)
	})

	t.Run("unsupported_language", func(t *testing.T) {
		cr := newDDI("a", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{
			Enabled:        true,
			TracerVersions: map[string]string{"cobol": "v1"},
		})
		errs := h.Validate(cr)
		require.Len(t, errs, 1)
		assert.Equal(t, "UnsupportedLanguage", errs[0].Reason)
		assert.Equal(t, "spec.config.apm.ddTraceVersions[cobol]", errs[0].Field)
	})

	t.Run("env_var_missing_dd_prefix", func(t *testing.T) {
		cr := newDDI("a", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{
			Enabled:       true,
			TracerConfigs: []corev1.EnvVar{{Name: "FOO", Value: "bar"}, {Name: "DD_SERVICE", Value: "svc"}},
		})
		errs := h.Validate(cr)
		require.Len(t, errs, 1)
		assert.Equal(t, "InvalidTracerConfig", errs[0].Reason)
		assert.Equal(t, "spec.config.apm.ddTraceConfigs[0].name", errs[0].Field)
	})
}

func TestAPMHandler_Handle_CreateUpdate_UpsertsStore(t *testing.T) {
	store := crstore.New()
	h := handlers.NewAPMHandler(handlers.Deps{CRStore: store})

	apm := &datadoghq.DatadogInstrumentationAPMConfig{
		Enabled:        true,
		TracerVersions: map[string]string{"java": "v1"},
		TracerConfigs:  []corev1.EnvVar{{Name: "DD_SERVICE", Value: "svc"}},
	}
	cr := newDDI("ddi-web", "default", "Deployment", "web", apm)

	status, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	assert.Equal(t, "APMReady", status.Type)
	assert.Equal(t, metav1.ConditionTrue, status.Status)
	assert.Equal(t, "SettingsApplied", status.Reason)

	entry, ok := store.GetAPM(crstore.WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"})
	require.True(t, ok)
	assert.True(t, entry.Enabled)
	assert.Equal(t, "v1", entry.TracerVersions["java"])
	assert.Equal(t, types.NamespacedName{Namespace: "default", Name: "ddi-web"}, entry.CR)
}

func TestAPMHandler_Handle_Delete_RemovesFromStore(t *testing.T) {
	store := crstore.New()
	h := handlers.NewAPMHandler(handlers.Deps{CRStore: store})

	apm := &datadoghq.DatadogInstrumentationAPMConfig{Enabled: true}
	cr := newDDI("ddi-web", "default", "Deployment", "web", apm)
	_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)

	status, err := h.Handle(context.Background(), instrumentation.EventDelete, cr)
	require.NoError(t, err)
	assert.Equal(t, "APMReady", status.Type)
	assert.Equal(t, metav1.ConditionUnknown, status.Status)
	assert.Equal(t, "SettingsRemoved", status.Reason)

	_, ok := store.GetAPM(crstore.WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"})
	assert.False(t, ok)
}

func TestAPMHandler_Handle_UnsupportedTarget_DoesNotWriteStore(t *testing.T) {
	store := crstore.New()
	h := handlers.NewAPMHandler(handlers.Deps{CRStore: store})

	cr := newDDI("ddi-job", "default", "CronJob", "nightly", &datadoghq.DatadogInstrumentationAPMConfig{Enabled: true})
	status, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionFalse, status.Status)
	assert.Equal(t, "UnsupportedTarget", status.Reason)

	_, ok := store.GetAPM(crstore.WorkloadKey{Kind: "CronJob", Namespace: "default", Name: "nightly"})
	assert.False(t, ok)
}

func TestAPMHandler_Handle_NilCR(t *testing.T) {
	h := handlers.NewAPMHandler(handlers.Deps{CRStore: crstore.New()})
	status, err := h.Handle(context.Background(), instrumentation.EventCreate, nil)
	require.NoError(t, err)
	assert.Equal(t, instrumentation.HandlerStatus{}, status)
}
