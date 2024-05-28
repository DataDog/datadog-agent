// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

// Package autoscaling implements the webhook that vertically scales applications
package autoscaling

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	admiv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

const (
	recommendationIDAnotation = "autoscaling.datadoghq.com/rec-id"

	webhookName     = "autoscaling"
	webhookEndpoint = "/autoscaling"

	requestType = "request"
	limitType   = "limit"
)

// Webhook implements the MutatingWebhook interface
type Webhook struct {
	name        string
	isEnabled   bool
	endpoint    string
	resources   []string
	operations  []admiv1.OperationType
	recommender workload.PatcherAdapter
}

// NewWebhook returns a new Webhook
func NewWebhook(recommender workload.PatcherAdapter) *Webhook {
	return &Webhook{
		name:        webhookName,
		isEnabled:   config.Datadog().GetBool("autoscaling.workload.enabled"),
		endpoint:    webhookEndpoint,
		resources:   []string{"pods"},
		operations:  []admiv1.OperationType{admiv1.Create},
		recommender: recommender,
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
func (w *Webhook) updateResources(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
	if len(pod.OwnerReferences) == 0 {
		return false, fmt.Errorf("no owner found for pod %s", pod.Name)
	}
	ownerRef := pod.OwnerReferences[0]
	if ownerRef.Kind == kubernetes.ReplicaSetKind {
		ownerRef.Kind = kubernetes.DeploymentKind
		ownerRef.Name = kubernetes.ParseDeploymentForReplicaSet(ownerRef.Name)
	}
	// ParseDeploymentForReplicaSet returns "" when the parsing fails
	if ownerRef.Name == "" {
		return false, fmt.Errorf("no owner found for pod %s", pod.Name)
	}

	recommendationID, recommendations, err := w.recommender.GetRecommendations(pod.Namespace, ownerRef)
	if err != nil || recommendationID == "" {
		return false, err
	}

	// Patching the pod with the recommendations
	injected := false
	if pod.Annotations[recommendationIDAnotation] != recommendationID {
		pod.Annotations[recommendationIDAnotation] = recommendationID
		injected = true
	}

	for _, reco := range recommendations {
		for i := range pod.Spec.Containers {
			cont := &pod.Spec.Containers[i]
			if cont.Name != reco.Name {
				continue
			}
			if cont.Resources.Limits == nil {
				cont.Resources.Limits = corev1.ResourceList{}
			}
			if cont.Resources.Requests == nil {
				cont.Resources.Requests = corev1.ResourceList{}
			}
			for resource, limit := range reco.Limits {
				if limit != cont.Resources.Limits[resource] {
					cont.Resources.Limits[resource] = limit
					injections.Set(limit.AsApproximateFloat64(), string(resource), ns, ownerRef.Name, cont.Name, limitType, recommendationID)
					injected = true
				}
			}
			for resource, request := range reco.Requests {
				if request != cont.Resources.Requests[resource] {
					cont.Resources.Requests[resource] = request
					injections.Set(request.AsApproximateFloat64(), string(resource), ns, ownerRef.Name, cont.Name, requestType, recommendationID)
					injected = true
				}
			}
			break
		}
	}

	return injected, nil
}
