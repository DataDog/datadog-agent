// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package webhook

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNewController(t *testing.T) {
	client := fake.NewSimpleClientset()
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	datadogConfig := config.NewMock(t)
	factory := informers.NewSharedInformerFactory(client, time.Duration(0))
	imageResolver := autoinstrumentation.NewImageResolver(nil)

	// V1
	controller := NewController(
		client,
		factory.Core().V1().Secrets(),
		factory.Admissionregistration(),
		factory.Admissionregistration(),
		func() bool { return true },
		make(<-chan struct{}),
		getV1Cfg(t),
		wmeta,
		nil,
		datadogConfig,
		nil,
		imageResolver,
	)

	assert.IsType(t, &ControllerV1{}, controller)

	// V1beta1
	controller = NewController(
		client,
		factory.Core().V1().Secrets(),
		factory.Admissionregistration(),
		factory.Admissionregistration(),
		func() bool { return true },
		make(<-chan struct{}),
		getV1beta1Cfg(t),
		wmeta,
		nil,
		datadogConfig,
		nil,
		imageResolver,
	)

	assert.IsType(t, &ControllerV1beta1{}, controller)
}

func TestAutoInstrumentation(t *testing.T) {
	tests := []struct {
		name        string
		pod         *corev1.Pod
		config      string
		expectPatch bool
	}{
		{
			name:   "disabled webhooks should not patch",
			config: "testdata/all_disabled.yaml",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"admission.datadoghq.com/enabled": "true",
					},
					Namespace: "foo",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "foo",
						},
					},
				},
			},
			expectPatch: false,
		},
		{
			name:   "disabled webhooks should not patch, even with targets",
			config: "testdata/all_disabled_targets.yaml",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						"admission.datadoghq.com/enabled": "true",
						"app":                             "billing-service",
					},
					Namespace: "foo",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "foo",
						},
					},
				},
			},
			expectPatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock config.
			mockConfig := configmock.NewFromFile(t, tt.config)

			// Create a mock meta.
			wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				fx.Supply(config.Params{}),
				fx.Provide(func() log.Component { return logmock.New(t) }),
				fx.Provide(func() config.Component { return config.NewMock(t) }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			// Create APM webhook.
			imageResolver := autoinstrumentation.NewImageResolver(nil)
			apm, err := generateAutoInstrumentationWebhook(wmeta, mockConfig, imageResolver)
			assert.NoError(t, err)

			// Create request.
			podJSON, err := json.Marshal(tt.pod)
			assert.NoError(t, err)
			t.Log(string(podJSON))
			request := &admission.Request{
				Object:    podJSON,
				Namespace: tt.pod.Namespace,
			}

			// Send request.
			f := apm.WebhookFunc()
			response := f(request)

			// Check if the patch is expected.
			emptyPatch := "null"
			if tt.expectPatch {
				assert.NotEqual(t, emptyPatch, string(response.Patch))
			} else {
				assert.Equal(t, emptyPatch, string(response.Patch))
			}
		})
	}
}
