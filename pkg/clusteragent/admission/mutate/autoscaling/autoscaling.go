// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

// Package autoscaling implements the webhook that vertically scales applications
package autoscaling

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	autoscaling "github.com/DataDog/datadog-agent/comp/autoscaling/workload/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/config"

	admiv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

const (
	webhookName     = "autoscaling"
	webhookEndpoint = "/autoscaling"
)

// Webhook implements the MutatingWebhook interface
type Webhook struct {
	name       string
	isEnabled  bool
	endpoint   string
	resources  []string
	operations []admiv1.OperationType
	patcher    autoscaling.Component
}

// NewWebhook returns a new Webhook
func NewWebhook(patcher autoscaling.Component) *Webhook {
	return &Webhook{
		name:       webhookName,
		isEnabled:  config.Datadog().GetBool("autoscaling.workload.enabled"),
		endpoint:   webhookEndpoint,
		resources:  []string{"pods"},
		operations: []admiv1.OperationType{admiv1.Create},
		patcher:    patcher,
	}
}

// Name returns the name of the webhook
func (w *Webhook) Name() string {
	return w.name
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
func (w *Webhook) Resources() []string {
	return w.resources
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *Webhook) Operations() []admiv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *Webhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	// Autoscaling does not work like others. Targets are selected through existence of DPA objects.
	// Hence, we need the equivalent of mutate unlabelled for this webhook.
	return nil, nil
}

// MutateFunc returns the function that mutates the resources
func (w *Webhook) MutateFunc() admission.WebhookFunc {
	return w.mutate
}

// mutate adds the DD_AGENT_HOST and DD_ENTITY_ID env vars to the pod template if they don't exist
func (w *Webhook) mutate(request *admission.MutateRequest) ([]byte, error) {
	return common.Mutate(request.Raw, request.Namespace, w.Name(), w.updateResources, request.DynamicClient)
}

// updateResource finds the owner of a pod, calls the recommender to retrieve the recommended CPU and Memory
// requests
func (w *Webhook) updateResources(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	return w.patcher.ApplyRecommendations(pod)
}
