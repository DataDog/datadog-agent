// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

// Package autoscaling implements the webhook that vertically scales applications
package autoscaling

import (
	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	webhookName     = "autoscaling"
	webhookEndpoint = "/autoscaling"
)

// Webhook implements the MutatingWebhook interface
type Webhook struct {
	name            string
	isEnabled       bool
	endpoint        string
	resources       map[string][]string
	operations      []admissionregistrationv1.OperationType
	matchConditions []admissionregistrationv1.MatchCondition
	patcher         workload.PodPatcher
}

// NewWebhook returns a new Webhook
func NewWebhook(patcher workload.PodPatcher) *Webhook {
	return &Webhook{
		name:            webhookName,
		isEnabled:       pkgconfigsetup.Datadog().GetBool("autoscaling.workload.enabled"),
		endpoint:        webhookEndpoint,
		resources:       map[string][]string{"": {"pods"}},
		operations:      []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		matchConditions: []admissionregistrationv1.MatchCondition{},
		patcher:         patcher,
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

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *Webhook) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *Webhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	// Autoscaling does not work like others. Targets are selected through existence of DPA objects.
	// Hence, we need the equivalent of mutate unlabelled for this webhook.
	return nil, nil
}

// MatchConditions returns the Match Conditions used for fine-grained
// request filtering
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	return w.matchConditions
}

// WebhookFunc returns the function that mutates the resources
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		return common.MutationResponse(mutatecommon.Mutate(request.Object, request.Namespace, w.Name(), w.updateResources, request.DynamicClient))
	}
}

// updateResources finds the owner of a pod, calls the recommender to retrieve the recommended CPU and Memory requests
func (w *Webhook) updateResources(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	return w.patcher.ApplyRecommendations(pod)
}
