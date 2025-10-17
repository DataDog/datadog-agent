// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package tagsfromlabels implements the webhook that injects DD_ENV,
// DD_VERSION, DD_SERVICE env vars into a pod template if needed
package tagsfromlabels

import (
	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

const webhookName = "standard_tags"

// Webhook is the webhook that injects DD_ENV, DD_VERSION, DD_SERVICE env vars
type Webhook struct {
	name            string
	isEnabled       bool
	endpoint        string
	resources       map[string][]string
	operations      []admissionregistrationv1.OperationType
	matchConditions []admissionregistrationv1.MatchCondition
	wmeta           workloadmeta.Component
	mutator         mutatecommon.Mutator
}

// NewWebhook returns a new Webhook
func NewWebhook(wmeta workloadmeta.Component, datadogConfig config.Component, mutator mutatecommon.Mutator) *Webhook {
	return &Webhook{
		name:            webhookName,
		isEnabled:       datadogConfig.GetBool("admission_controller.inject_tags.enabled"),
		endpoint:        datadogConfig.GetString("admission_controller.inject_tags.endpoint"),
		resources:       map[string][]string{"": {"pods"}},
		operations:      []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		matchConditions: []admissionregistrationv1.MatchCondition{},
		wmeta:           wmeta,
		mutator:         mutator,
	}
}

// Name returns the name of the webhook
func (w *Webhook) Name() string {
	return w.name
}

// WebhookType returns the type of the webhook
func (w *Webhook) WebhookType() common.WebhookType {
	return common.MutatingWebhook
}

// IsEnabled returns whether the webhook is enabled
func (w *Webhook) IsEnabled() bool {
	return w.isEnabled
}

// Endpoint returns the endpoint of the webhook
func (w *Webhook) Endpoint() string {
	return w.endpoint
}

// Resources returns the kubernetes resources for which the webhook should
// be invoked
func (w *Webhook) Resources() map[string][]string {
	return w.resources
}

// Timeout returns the timeout for the webhook
func (w *Webhook) Timeout() int32 {
	return 0
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *Webhook) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return common.DefaultLabelSelectors(useNamespaceSelector)
}

// MatchConditions returns the Match Conditions used for fine-grained
// request filtering
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	return w.matchConditions
}

// WebhookFunc returns the function that mutates the resources
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Object, request.Namespace, w.Name(), func(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
			// Adds the DD_ENV, DD_VERSION, DD_SERVICE env vars to the pod template from pod and higher-level resource labels.
			return w.inject(pod, ns, dc)
		}, request.DynamicClient))
	}
}

// inject is a helper method to call the underlying injector directly from the webook. This is useful for testing. All
// the logic must be in the injector itself and we should consider refactoring this method out.
func (w *Webhook) inject(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	return w.mutator.MutatePod(pod, ns, dc)
}
