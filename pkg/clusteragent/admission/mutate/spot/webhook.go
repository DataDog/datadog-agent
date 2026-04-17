// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package spot implements the admission webhook that assigns pods to spot instances.
package spot

import (
	"encoding/json"
	"fmt"

	admiv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	clusterspot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/spot"
)

const (
	webhookName     = "spot-scheduling"
	webhookEndpoint = "/spot-scheduling"
)

// Webhook implements the MutatingWebhook interface for spot scheduling.
type Webhook struct {
	name            string
	isEnabled       bool
	endpoint        string
	resources       map[string][]string
	operations      []admissionregistrationv1.OperationType
	matchConditions []admissionregistrationv1.MatchCondition
	handler         clusterspot.PodHandler
}

// NewWebhook returns a new spot-scheduling Webhook.
func NewWebhook(datadogConfig config.Component, handler clusterspot.PodHandler) *Webhook {
	return &Webhook{
		name:            webhookName,
		isEnabled:       datadogConfig.GetBool("autoscaling.cluster.spot.enabled"),
		endpoint:        webhookEndpoint,
		resources:       map[string][]string{"": {"pods"}},
		operations:      []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Delete},
		matchConditions: []admissionregistrationv1.MatchCondition{},
		handler:         handler,
	}
}

// Name returns the name of the webhook.
func (w *Webhook) Name() string {
	return w.name
}

// WebhookType returns the type of the webhook.
func (w *Webhook) WebhookType() common.WebhookType {
	return common.MutatingWebhook
}

// IsEnabled returns whether the webhook is enabled.
func (w *Webhook) IsEnabled() bool {
	return w.isEnabled
}

// Endpoint returns the endpoint of the webhook.
func (w *Webhook) Endpoint() string {
	return w.endpoint
}

// Resources returns the kubernetes resources for which the webhook should be invoked.
func (w *Webhook) Resources() map[string][]string {
	return w.resources
}

// Timeout returns the timeout for the webhook.
func (w *Webhook) Timeout() int32 {
	return 0
}

// Operations returns the operations on the resources specified for which the webhook should be invoked.
func (w *Webhook) Operations() []admissionregistrationv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook should be invoked.
func (w *Webhook) LabelSelectors(_ bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	// Spot scheduling targets are selected through the spot-enabled label on workloads, not pods.
	// Hence, we need the equivalent of mutate unlabelled for this webhook.
	return nil, nil
}

// MatchConditions returns the Match Conditions used for fine-grained request filtering.
func (w *Webhook) MatchConditions() []admissionregistrationv1.MatchCondition {
	return w.matchConditions
}

// WebhookFunc returns the function that mutates the resources.
func (w *Webhook) WebhookFunc() admission.WebhookFunc {
	return func(request *admission.Request) *admiv1.AdmissionResponse {
		if request.DryRun != nil && *request.DryRun {
			return &admiv1.AdmissionResponse{Allowed: true}
		}
		switch request.Operation {
		case admissionregistrationv1.Create:
			return w.podCreated(request)
		case admissionregistrationv1.Delete:
			return w.podDeleted(request)
		default:
			return &admiv1.AdmissionResponse{Allowed: true}
		}
	}
}

func (w *Webhook) podCreated(request *admission.Request) *admiv1.AdmissionResponse {
	return common.MutationResponse(mutatecommon.Mutate(request.Object, request.Namespace, w.Name(),
		func(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
			return w.handler.PodCreated(pod)
		}, request.DynamicClient))
}

func (w *Webhook) podDeleted(request *admission.Request) *admiv1.AdmissionResponse {
	var pm metav1.PartialObjectMetadata
	if err := json.Unmarshal(request.OldObject, &pm); err != nil {
		return common.MutationResponse(nil, fmt.Errorf("failed to decode raw object: %v", err))
	}
	pod := &corev1.Pod{ObjectMeta: pm.ObjectMeta}
	w.handler.PodDeleted(pod)
	return &admiv1.AdmissionResponse{Allowed: true}
}
