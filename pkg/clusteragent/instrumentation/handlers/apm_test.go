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
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/ssi/crstore"
)

func newAPMDDI(crName, crNamespace, targetKind, targetName string, apm *datadoghq.DatadogInstrumentationAPMConfig) *datadoghq.DatadogInstrumentation {
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

func TestAPMHandlerHasSection(t *testing.T) {
	h := NewAPMHandler(&Deps{})

	tests := []struct {
		name string
		cr   *datadoghq.DatadogInstrumentation
		want bool
	}{
		{
			name: "nil cr",
			cr:   nil,
			want: false,
		},
		{
			name: "nil apm section",
			cr:   newAPMDDI("ddi", "default", "Deployment", "web", nil),
			want: false,
		},
		{
			name: "apm section present",
			cr:   newAPMDDI("ddi", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, h.HasSection(tt.cr))
		})
	}
}

func TestAPMHandlerSupportsTarget(t *testing.T) {
	h := NewAPMHandler(&Deps{})

	tests := []struct {
		name string
		kind string
		want bool
	}{
		{name: "deployment", kind: "Deployment", want: true},
		{name: "statefulset", kind: "StatefulSet", want: true},
		{name: "daemonset", kind: "DaemonSet", want: true},
		{name: "job", kind: "Job", want: true},
		{name: "cronjob", kind: "CronJob", want: true},
		{name: "service", kind: "Service", want: false},
		{name: "pod", kind: "Pod", want: false},
		{name: "empty kind", kind: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, h.SupportsTarget(autoscalingv2.CrossVersionObjectReference{Kind: tt.kind}))
		})
	}
}

func TestAPMHandlerValidate(t *testing.T) {
	h := NewAPMHandler(&Deps{})

	type expectedValidationError struct {
		reason string
		field  string
	}

	tests := []struct {
		name string
		cr   *datadoghq.DatadogInstrumentation
		want []expectedValidationError
	}{
		{
			name: "nil cr",
			cr:   nil,
			want: nil,
		},
		{
			name: "nil apm section",
			cr:   newAPMDDI("ddi", "default", "Deployment", "web", nil),
			want: nil,
		},
		{
			name: "valid config",
			cr: newAPMDDI("ddi", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{
				Enabled:        true,
				TracerVersions: map[string]string{"java": "v1", "python": "v4"},
				TracerConfigs:  []corev1.EnvVar{{Name: "DD_SERVICE", Value: "web"}},
			}),
			want: nil,
		},
		{
			name: "unsupported target is checked by controller",
			cr: newAPMDDI("ddi", "default", "Service", "web", &datadoghq.DatadogInstrumentationAPMConfig{
				Enabled:        true,
				TracerVersions: map[string]string{"java": "v1"},
				TracerConfigs:  []corev1.EnvVar{{Name: "DD_SERVICE", Value: "web"}},
			}),
			want: nil,
		},
		{
			name: "unsupported tracer language",
			cr: newAPMDDI("ddi", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{
				Enabled:        true,
				TracerVersions: map[string]string{"cobol": "v1"},
			}),
			want: []expectedValidationError{{
				reason: reasonAPMUnsupportedLang,
				field:  "spec.config.apm.ddTraceVersions[cobol]",
			}},
		},
		{
			name: "invalid tracer config env var name",
			cr: newAPMDDI("ddi", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{
				Enabled:       true,
				TracerConfigs: []corev1.EnvVar{{Name: "NOT_DD", Value: "web"}},
			}),
			want: []expectedValidationError{{
				reason: reasonAPMInvalidConfig,
				field:  "spec.config.apm.ddTraceConfigs[0].name",
			}},
		},
		{
			name: "multiple apm validation errors",
			cr: newAPMDDI("ddi", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{
				Enabled:        true,
				TracerVersions: map[string]string{"cobol": "v1"},
				TracerConfigs:  []corev1.EnvVar{{Name: "NOT_DD", Value: "web"}},
			}),
			want: []expectedValidationError{
				{
					reason: reasonAPMUnsupportedLang,
					field:  "spec.config.apm.ddTraceVersions[cobol]",
				},
				{
					reason: reasonAPMInvalidConfig,
					field:  "spec.config.apm.ddTraceConfigs[0].name",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := h.Validate(tt.cr)
			actual := make([]expectedValidationError, 0, len(errs))
			for _, err := range errs {
				require.Equal(t, apmReadyConditionType, err.Type)
				require.Equal(t, h.Name(), err.HandlerName)
				actual = append(actual, expectedValidationError{
					reason: err.Reason,
					field:  err.Field,
				})
			}
			require.ElementsMatch(t, tt.want, actual)
		})
	}
}

func TestAPMHandlerHandle(t *testing.T) {
	workload := crstore.WorkloadTarget{Kind: "Deployment", Namespace: "default", Name: "web"}
	unsupportedWorkload := crstore.WorkloadTarget{Kind: "Service", Namespace: "default", Name: "web"}
	validCR := newAPMDDI("ddi-web", "default", "Deployment", "web", &datadoghq.DatadogInstrumentationAPMConfig{
		Enabled:        true,
		TracerVersions: map[string]string{"java": "v1"},
		TracerConfigs:  []corev1.EnvVar{{Name: "DD_SERVICE", Value: "web"}},
	})
	unsupportedCR := newAPMDDI("ddi-svc", "default", "Service", "web", &datadoghq.DatadogInstrumentationAPMConfig{Enabled: true})

	tests := []struct {
		name        string
		store       *crstore.Store
		event       instrumentation.EventType
		cr          *datadoghq.DatadogInstrumentation
		setup       func(t *testing.T, h *APMHandler)
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantErr     bool
		assertStore func(t *testing.T, store *crstore.Store)
	}{
		{
			name:       "create upserts store",
			store:      crstore.New(),
			event:      instrumentation.EventCreate,
			cr:         validCR,
			wantStatus: metav1.ConditionTrue,
			wantReason: reasonAPMConfigured,
			assertStore: func(t *testing.T, store *crstore.Store) {
				entry, ok := store.GetAPM(workload)
				require.True(t, ok)
				require.True(t, entry.Enabled)
				require.Equal(t, "v1", entry.TracerVersions["java"])
				require.Equal(t, []corev1.EnvVar{{Name: "DD_SERVICE", Value: "web"}}, entry.TracerConfigs)
				require.Equal(t, types.NamespacedName{Namespace: "default", Name: "ddi-web"}, entry.CR)
			},
		},
		{
			name:  "delete removes store entry",
			store: crstore.New(),
			event: instrumentation.EventDelete,
			cr:    validCR,
			setup: func(t *testing.T, h *APMHandler) {
				_, err := h.Handle(context.Background(), instrumentation.EventCreate, validCR)
				require.NoError(t, err)
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: reasonAPMDeleted,
			assertStore: func(t *testing.T, store *crstore.Store) {
				_, ok := store.GetAPM(workload)
				require.False(t, ok)
			},
		},
		{
			name:       "unsupported target does not write store",
			store:      crstore.New(),
			event:      instrumentation.EventCreate,
			cr:         unsupportedCR,
			wantStatus: metav1.ConditionFalse,
			wantReason: reasonAPMUnsupportedTarget,
			assertStore: func(t *testing.T, store *crstore.Store) {
				_, ok := store.GetAPM(unsupportedWorkload)
				require.False(t, ok)
			},
		},
		{
			name:       "nil cr reports missing resource",
			store:      crstore.New(),
			event:      instrumentation.EventCreate,
			cr:         nil,
			wantStatus: metav1.ConditionUnknown,
			wantReason: "MissingResource",
		},
		{
			name:       "nil store returns error",
			store:      nil,
			event:      instrumentation.EventCreate,
			cr:         validCR,
			wantStatus: metav1.ConditionFalse,
			wantReason: reasonAPMStoreUnavailable,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewAPMHandler(&Deps{APMStore: tt.store})
			if tt.setup != nil {
				tt.setup(t, h)
			}

			status, err := h.Handle(context.Background(), tt.event, tt.cr)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, apmReadyConditionType, status.Type)
			require.Equal(t, tt.wantStatus, status.Status)
			require.Equal(t, tt.wantReason, status.Reason)
			if tt.assertStore != nil {
				tt.assertStore(t, tt.store)
			}
		})
	}
}
