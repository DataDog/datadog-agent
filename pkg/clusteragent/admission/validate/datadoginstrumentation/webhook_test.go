// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package datadoginstrumentation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// mockHandler is a test double for instrumentation.Handler.
type mockHandler struct {
	name           string
	hasSection     func(*datadoghq.DatadogInstrumentation) bool
	supportsTarget func(autoscalingv2.CrossVersionObjectReference) bool
	validate       func(*datadoghq.DatadogInstrumentation) []instrumentation.ValidationError
}

func (m *mockHandler) Name() string { return m.name }

func (m *mockHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return m.hasSection(cr)
}

func (m *mockHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	return m.supportsTarget(ref)
}

func (m *mockHandler) Handle(_ context.Context, _ instrumentation.EventType, _ *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
	return instrumentation.HandlerStatus{}, nil
}

func (m *mockHandler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	return m.validate(cr)
}

// test helpers

func buildCR(name, namespace, targetKind, targetName string, checks []datadoghq.DatadogInstrumentationCheckConfig) *datadoghq.DatadogInstrumentation {
	return &datadoghq.DatadogInstrumentation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogInstrumentation",
			APIVersion: datadoghq.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
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

func crAsUnstructured(t *testing.T, cr *datadoghq.DatadogInstrumentation) *unstructured.Unstructured {
	t.Helper()
	raw, err := json.Marshal(cr)
	require.NoError(t, err)
	var u unstructured.Unstructured
	require.NoError(t, json.Unmarshal(raw, &u))
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   datadoghq.GroupVersion.Group,
		Version: datadoghq.GroupVersion.Version,
		Kind:    "DatadogInstrumentation",
	})
	return &u
}

func marshalCR(t *testing.T, cr *datadoghq.DatadogInstrumentation) []byte {
	t.Helper()
	raw, err := json.Marshal(cr)
	require.NoError(t, err)
	return raw
}

func newRequest(t *testing.T, op admissionregistrationv1.OperationType, ns string, cr *datadoghq.DatadogInstrumentation) *admission.Request {
	t.Helper()
	return &admission.Request{
		Name:      cr.Name,
		Namespace: ns,
		Operation: op,
		Object:    marshalCR(t, cr),
	}
}

func newUpdateRequest(t *testing.T, ns string, oldCR, newCR *datadoghq.DatadogInstrumentation) *admission.Request {
	t.Helper()
	return &admission.Request{
		Name:      newCR.Name,
		Namespace: ns,
		Operation: admissionregistrationv1.Update,
		Object:    marshalCR(t, newCR),
		OldObject: marshalCR(t, oldCR),
	}
}

func fakeInformerFactory(t *testing.T, crs ...*datadoghq.DatadogInstrumentation) dynamicinformer.DynamicSharedInformerFactory {
	t.Helper()
	objs := make([]runtime.Object, 0, len(crs))
	for _, cr := range crs {
		objs = append(objs, crAsUnstructured(t, cr))
	}
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		instrumentation.DatadogInstrumentationGVR: "DatadogInstrumentationList",
	}, objs...)
	factory := dynamicinformer.NewDynamicSharedInformerFactory(client, 0)
	// Pre-create the informer so it is included in Start/WaitForCacheSync.
	factory.ForResource(instrumentation.DatadogInstrumentationGVR)
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)
	return factory
}

func newTestWebhook(t *testing.T, factory dynamicinformer.DynamicSharedInformerFactory, handlers ...instrumentation.Handler) *Webhook {
	cfg := config.NewMock(t)
	cfg.SetInTest("instrumentation_crd_controller.enabled", true)
	return NewWebhook(cfg, handlers, factory)
}

func defaultChecks() []datadoghq.DatadogInstrumentationCheckConfig {
	return []datadoghq.DatadogInstrumentationCheckConfig{{Integration: "redisdb"}}
}

func alwaysHasSection(_ *datadoghq.DatadogInstrumentation) bool { return true }
func neverHasSection(_ *datadoghq.DatadogInstrumentation) bool  { return false }
func alwaysSupports(_ autoscalingv2.CrossVersionObjectReference) bool {
	return true
}
func noValidationErrors(_ *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	return nil
}

func TestWebhookInterface(t *testing.T) {
	w := newTestWebhook(t, fakeInformerFactory(t))
	assert.Equal(t, "datadog_instrumentation_validation", w.Name())
	assert.Equal(t, common.WebhookType(common.ValidatingWebhook), w.WebhookType())
	assert.True(t, w.IsEnabled())
	assert.Equal(t, "/datadog-instrumentation-validation", w.Endpoint())
	assert.Equal(t, []common.WebhookResourceRule{{APIGroup: "datadoghq.com", APIVersion: "v1alpha1", Resources: []string{"datadoginstrumentations"}}}, w.Resources())
	assert.Equal(t, []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update}, w.Operations())
	nsSelector, objSelector := w.LabelSelectors(false)
	assert.Nil(t, nsSelector)
	assert.Nil(t, objSelector)
	assert.Nil(t, w.MatchConditions())
	assert.Equal(t, int32(0), w.Timeout())
}

func TestValidate(t *testing.T) {
	const ns = "test-ns"

	handlerWithSection := &mockHandler{
		name:           "test-handler",
		hasSection:     alwaysHasSection,
		supportsTarget: alwaysSupports,
		validate:       noValidationErrors,
	}

	tests := []struct {
		name        string
		cr          *datadoghq.DatadogInstrumentation
		oldCR       *datadoghq.DatadogInstrumentation
		existingCRs []*datadoghq.DatadogInstrumentation
		handlers    []instrumentation.Handler
		operation   admissionregistrationv1.OperationType
		wantAllowed bool
		wantMsg     string
	}{
		{
			name:        "valid CR with no handlers passes",
			cr:          buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			handlers:    nil,
			operation:   admissionregistrationv1.Create,
			wantAllowed: true,
		},
		{
			name:        "valid CR with handler passes all stages",
			cr:          buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			handlers:    []instrumentation.Handler{handlerWithSection},
			operation:   admissionregistrationv1.Create,
			wantAllowed: true,
		},
		{
			name: "duplicate targetRef on create is rejected",
			cr:   buildCR("di-new", ns, "Deployment", "my-app", defaultChecks()),
			existingCRs: []*datadoghq.DatadogInstrumentation{
				buildCR("di-existing", ns, "Deployment", "my-app", nil),
			},
			handlers:    nil,
			operation:   admissionregistrationv1.Create,
			wantAllowed: false,
			wantMsg:     `DatadogInstrumentation "di-existing" in namespace "test-ns" already targets Deployment/my-app`,
		},
		{
			name:  "duplicate targetRef on update of the same CR is allowed",
			cr:    buildCR("di-existing", ns, "Deployment", "my-app", defaultChecks()),
			oldCR: buildCR("di-existing", ns, "Deployment", "my-app", nil),
			existingCRs: []*datadoghq.DatadogInstrumentation{
				buildCR("di-existing", ns, "Deployment", "my-app", nil),
			},
			handlers:    nil,
			operation:   admissionregistrationv1.Update,
			wantAllowed: true,
		},
		{
			name:  "changing targetRef name on update is rejected",
			cr:    buildCR("di-1", ns, "Deployment", "other-app", defaultChecks()),
			oldCR: buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			existingCRs: []*datadoghq.DatadogInstrumentation{
				buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			},
			operation:   admissionregistrationv1.Update,
			wantAllowed: false,
			wantMsg:     "spec.targetRef is immutable: cannot change from Deployment/my-app to Deployment/other-app",
		},
		{
			name:  "changing targetRef kind on update is rejected",
			cr:    buildCR("di-1", ns, "DaemonSet", "my-app", defaultChecks()),
			oldCR: buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			existingCRs: []*datadoghq.DatadogInstrumentation{
				buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			},
			operation:   admissionregistrationv1.Update,
			wantAllowed: false,
			wantMsg:     "spec.targetRef is immutable: cannot change from Deployment/my-app to DaemonSet/my-app",
		},
		{
			name: "different target kind is not a duplicate",
			cr:   buildCR("di-new", ns, "DaemonSet", "my-app", defaultChecks()),
			existingCRs: []*datadoghq.DatadogInstrumentation{
				buildCR("di-existing", ns, "Deployment", "my-app", nil),
			},
			handlers:    nil,
			operation:   admissionregistrationv1.Create,
			wantAllowed: true,
		},
		{
			name: "different target name is not a duplicate",
			cr:   buildCR("di-new", ns, "Deployment", "other-app", defaultChecks()),
			existingCRs: []*datadoghq.DatadogInstrumentation{
				buildCR("di-existing", ns, "Deployment", "my-app", nil),
			},
			handlers:    nil,
			operation:   admissionregistrationv1.Create,
			wantAllowed: true,
		},
		{
			name: "unsupported target kind is rejected by handler",
			cr:   buildCR("di-1", ns, "UnknownKind", "my-app", defaultChecks()),
			handlers: []instrumentation.Handler{&mockHandler{
				name:       "rejecting-handler",
				hasSection: alwaysHasSection,
				supportsTarget: func(_ autoscalingv2.CrossVersionObjectReference) bool {
					return false
				},
				validate: noValidationErrors,
			}},
			operation:   admissionregistrationv1.Create,
			wantAllowed: false,
			wantMsg:     `handler "rejecting-handler" does not support target kind "UnknownKind"`,
		},
		{
			name: "handler validation errors are collected and returned",
			cr:   buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			handlers: []instrumentation.Handler{&mockHandler{
				name:           "validating-handler",
				hasSection:     alwaysHasSection,
				supportsTarget: alwaysSupports,
				validate: func(_ *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
					return []instrumentation.ValidationError{
						{Type: "Invalid", Reason: "BadField", Message: "field X is invalid"},
						{Type: "Invalid", Reason: "BadField", Field: "spec.config", Message: "must not be empty"},
					}
				},
			}},
			operation:   admissionregistrationv1.Create,
			wantAllowed: false,
			wantMsg:     "field X is invalid; spec.config: must not be empty",
		},
		{
			name: "handler with no matching section is skipped",
			cr:   buildCR("di-1", ns, "Deployment", "my-app", nil),
			handlers: []instrumentation.Handler{&mockHandler{
				name:           "skipped-handler",
				hasSection:     neverHasSection,
				supportsTarget: alwaysSupports,
				validate: func(_ *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
					return []instrumentation.ValidationError{{Message: "should not reach"}}
				},
			}},
			operation:   admissionregistrationv1.Create,
			wantAllowed: true,
		},
		{
			name: "multi-handler: first passes, second fails SupportsTarget",
			cr:   buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			handlers: []instrumentation.Handler{
				&mockHandler{
					name:           "passing-handler",
					hasSection:     alwaysHasSection,
					supportsTarget: alwaysSupports,
					validate:       noValidationErrors,
				},
				&mockHandler{
					name:       "rejecting-handler",
					hasSection: alwaysHasSection,
					supportsTarget: func(_ autoscalingv2.CrossVersionObjectReference) bool {
						return false
					},
					validate: noValidationErrors,
				},
			},
			operation:   admissionregistrationv1.Create,
			wantAllowed: false,
			wantMsg:     `handler "rejecting-handler" does not support target kind "Deployment"`,
		},
		{
			name: "multi-handler: first passes, second fails Validate",
			cr:   buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			handlers: []instrumentation.Handler{
				&mockHandler{
					name:           "passing-handler",
					hasSection:     alwaysHasSection,
					supportsTarget: alwaysSupports,
					validate:       noValidationErrors,
				},
				&mockHandler{
					name:           "failing-handler",
					hasSection:     alwaysHasSection,
					supportsTarget: alwaysSupports,
					validate: func(_ *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
						return []instrumentation.ValidationError{{Message: "bad config"}}
					},
				},
			},
			operation:   admissionregistrationv1.Create,
			wantAllowed: false,
			wantMsg:     "bad config",
		},
		{
			name: "multi-handler: first skipped (no section), second fails",
			cr:   buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			handlers: []instrumentation.Handler{
				&mockHandler{
					name:       "skipped-handler",
					hasSection: neverHasSection,
					supportsTarget: func(_ autoscalingv2.CrossVersionObjectReference) bool {
						return false
					},
					validate: func(_ *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
						return []instrumentation.ValidationError{{Message: "should not reach"}}
					},
				},
				&mockHandler{
					name:           "failing-handler",
					hasSection:     alwaysHasSection,
					supportsTarget: alwaysSupports,
					validate: func(_ *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
						return []instrumentation.ValidationError{{Message: "second handler failed"}}
					},
				},
			},
			operation:   admissionregistrationv1.Create,
			wantAllowed: false,
			wantMsg:     "second handler failed",
		},
		{
			name: "multi-handler: both pass",
			cr:   buildCR("di-1", ns, "Deployment", "my-app", defaultChecks()),
			handlers: []instrumentation.Handler{
				&mockHandler{
					name:           "handler-a",
					hasSection:     alwaysHasSection,
					supportsTarget: alwaysSupports,
					validate:       noValidationErrors,
				},
				&mockHandler{
					name:           "handler-b",
					hasSection:     alwaysHasSection,
					supportsTarget: alwaysSupports,
					validate:       noValidationErrors,
				},
			},
			operation:   admissionregistrationv1.Create,
			wantAllowed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factory := fakeInformerFactory(t, tc.existingCRs...)
			w := newTestWebhook(t, factory, tc.handlers...)

			var req *admission.Request
			if tc.oldCR != nil {
				req = newUpdateRequest(t, ns, tc.oldCR, tc.cr)
			} else {
				req = newRequest(t, tc.operation, ns, tc.cr)
			}

			resp := w.validate(req)
			assert.Equal(t, tc.wantAllowed, resp.Allowed)
			if !tc.wantAllowed {
				require.NotNil(t, resp.Result)
				assert.Contains(t, resp.Result.Message, tc.wantMsg)
			}
		})
	}
}

func TestWebhookFuncReturnsAdmissionResponse(t *testing.T) {
	w := newTestWebhook(t, fakeInformerFactory(t))
	cr := buildCR("di-1", "ns", "Deployment", "app", nil)
	req := newRequest(t, admissionregistrationv1.Create, "ns", cr)
	fn := w.WebhookFunc()
	resp := fn(req)
	assert.IsType(t, &admiv1.AdmissionResponse{}, resp)
	assert.True(t, resp.Allowed)
}
