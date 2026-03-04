// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

// Package ncclprofiler implements the admission webhook that injects the NCCL
// profiler plugin (libnccl-profiler-inspector.so) into GPU training pods.
// Users opt in with the label: admission.datadoghq.com/nccl-profiler.enabled=true
package ncclprofiler

import (
	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	webhookName     = "nccl_profiler"
	webhookEndpoint = "/inject-nccl-profiler"

	// EnabledLabel is the pod label users set to opt in to NCCL plugin injection.
	EnabledLabel = "admission.datadoghq.com/nccl-profiler.enabled"
)

var (
	webhookResources  = map[string][]string{"": {"pods"}}
	webhookOperations = []admissionregistrationv1.OperationType{admissionregistrationv1.Create}
)

// Webhook injects the NCCL profiler plugin into GPU training pods.
type Webhook struct {
	isEnabled     bool
	injectorImage string
}

// NewWebhook creates a new NCCL profiler webhook from agent config.
// The injector_image config is required when enabled; if empty the webhook
// is disabled with a warning so pods are not mutated with a broken image ref.
func NewWebhook(datadogConfig config.Component) *Webhook {
	enabled := datadogConfig.GetBool("admission_controller.nccl_profiler.enabled")
	image := datadogConfig.GetString("admission_controller.nccl_profiler.injector_image")
	if enabled && image == "" {
		log.Errorf("NCCL profiler webhook is enabled but admission_controller.nccl_profiler.injector_image is not set; disabling webhook")
		enabled = false
	}
	return &Webhook{
		isEnabled:     enabled,
		injectorImage: image,
	}
}

// Name returns the name of the webhook.
func (w *Webhook) Name() string { return webhookName }

// WebhookType returns the type of the webhook.
func (w *Webhook) WebhookType() common.WebhookType { return common.MutatingWebhook }

// IsEnabled returns whether the webhook is enabled.
func (w *Webhook) IsEnabled() bool { return w.isEnabled }

// Endpoint returns the HTTP endpoint of the webhook.
func (w *Webhook) Endpoint() string { return webhookEndpoint }

// Resources returns the Kubernetes resources for which the webhook is invoked.
func (w *Webhook) Resources() map[string][]string { return webhookResources }

// Operations returns the operations for which the webhook is invoked.
func (w *Webhook) Operations() []admissionregistrationv1.OperationType { return webhookOperations }

// LabelSelectors returns the label selectors for the webhook.
// The object selector targets pods with the opt-in label; namespace selector is unrestricted.
func (w *Webhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return nil, &metav1.LabelSelector{
		MatchLabels: map[string]string{EnabledLabel: "true"},
	}
}

// MatchConditions returns the match conditions for the webhook (none required).
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition { return nil }

// Timeout returns the admission webhook timeout in seconds (0 = use cluster default).
func (w *Webhook) Timeout() int32 { return 0 }

// WebhookFunc returns the function that mutates pods on admission.
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(
			request.Object,
			request.Namespace,
			w.Name(),
			func(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
				log.Debugf("Injecting NCCL profiler plugin into pod %s", mutatecommon.PodString(pod))
				return mutatePod(pod, w.injectorImage)
			},
			request.DynamicClient,
		))
	}
}
