// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package autoscaling implements the webhook that vertically scales applications
package autoscaling

import (
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/controllers/webhook"
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
	webhookName = "autoscaling"
)

type wh struct {
	name        string
	isEnabled   bool
	endpoint    string
	resources   []string
	operations  []admiv1.OperationType
	recommender workload.PatcherAdapter
}

// NewWebhook returns a new Webhook
func NewWebhook(recommender workload.PatcherAdapter) webhook.MutatingWebhook {
	return &wh{
		name:        webhookName,
		isEnabled:   config.Datadog.GetBool("admission_controller.autoscaling.enabled"),
		endpoint:    config.Datadog.GetString("admission_controller.autoscaling.endpoint"),
		resources:   []string{"pods"},
		operations:  []admiv1.OperationType{admiv1.Create},
		recommender: recommender,
	}
}

// Name returns the name of the webhook
func (w *wh) Name() string {
	return w.name
}

// IsEnabled returns whether the webhook is enabled
func (w *wh) IsEnabled() bool {
	return w.isEnabled
}

// Endpoint returns the endpoint of the webhook
func (w *wh) Endpoint() string {
	return w.endpoint
}

// Resources returns the kubernetes resources for which the webhook should
// be invoked
func (w *wh) Resources() []string {
	return w.resources
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *wh) Operations() []admiv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *wh) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return common.DefaultLabelSelectors(useNamespaceSelector)
}

// MutateFunc returns the function that mutates the resources
func (w *wh) MutateFunc() admission.WebhookFunc {
	return w.mutate
}

// mutate adds the DD_AGENT_HOST and DD_ENTITY_ID env vars to the pod template if they don't exist
func (w *wh) mutate(request *admission.MutateRequest) ([]byte, error) {
	return common.Mutate(request.Raw, request.Namespace, w.Name(), w.updateResources, request.DynamicClient)
}

// updateResource finds the owner of a pod, calls the recommender to retrieve the recommended CPU and Memory
// requests
func (w *wh) updateResources(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	ownerRef := pod.OwnerReferences[0]
	if ownerRef.Kind == kubernetes.ReplicaSetKind {
		ownerRef.Kind = kubernetes.DeploymentKind
		ownerRef.Name = kubernetes.ParseDeploymentForReplicaSet(ownerRef.Name)
	}

	recommendations, err := w.recommender.GetRecommendations(pod.Namespace, ownerRef)
	if err != nil {
		return false, err
	}

	injected := false
	for _, reco := range recommendations {
		for i := range pod.Spec.Containers {
			cont := &pod.Spec.Containers[i]
			if cont.Name != reco.Name {
				continue
			}
			for resource, limit := range reco.Limits {
				if limit != cont.Resources.Limits[resource] {
					cont.Resources.Limits[resource] = limit
					injected = true
				}
			}
			for resource, request := range reco.Requests {
				if request != cont.Resources.Requests[resource] {
					cont.Resources.Requests[resource] = request
					injected = true
				}
			}
			break
		}
	}

	return injected, nil
}
