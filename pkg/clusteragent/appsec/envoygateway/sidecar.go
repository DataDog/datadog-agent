// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package envoygateway

import (
	"context"
	"fmt"
	"slices"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/sidecar"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	// owningGatewayNameLabel is the label set by Envoy Gateway on proxy pods
	// to identify which Gateway owns the pod.
	owningGatewayNameLabel = "gateway.envoyproxy.io/owning-gateway-name"
	// owningGatewayNamespaceLabel is the label set by Envoy Gateway on proxy pods
	// to identify which namespace the owning Gateway belongs to.
	owningGatewayNamespaceLabel = "gateway.envoyproxy.io/owning-gateway-namespace"
)

var podGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

var _ appsecconfig.SidecarInjectionPattern = (*envoyGatewaySidecarPattern)(nil)

// envoyGatewaySidecarPattern wraps the base pattern for SIDECAR mode.
// It provides pod mutation logic to inject the appsec processor sidecar.
type envoyGatewaySidecarPattern struct {
	*envoyGatewayInjectionPattern
}

func (e *envoyGatewaySidecarPattern) ShouldMutatePod(pod *corev1.Pod) bool {
	// Check if sidecar already exists
	if sidecar.HasProcessorSidecar(pod) {
		e.logger.Debugf("Pod %s already has appsec processor sidecar", mutatecommon.PodString(pod))
		return false
	}

	// Check that the pod is owned by an Envoy Gateway proxy
	gatewayName := pod.Labels[owningGatewayNameLabel]
	return gatewayName != ""
}

func (e *envoyGatewaySidecarPattern) IsNamespaceEligible(string) bool {
	// We want to inject sidecar in all namespaces
	return true
}

func (e *envoyGatewaySidecarPattern) PodDeleted(pod *corev1.Pod, ns string, _ dynamic.Interface) (bool, error) {
	gatewayName := pod.Labels[owningGatewayNameLabel]
	gatewayNamespace := pod.Labels[owningGatewayNamespaceLabel]

	selector := labels.SelectorFromSet(map[string]string{
		owningGatewayNameLabel:      gatewayName,
		owningGatewayNamespaceLabel: gatewayNamespace,
	})

	// Check that this is the last pod of this gateway
	lst, err := e.client.Resource(podGVR).Namespace(ns).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})

	if err != nil {
		return false, fmt.Errorf("error listing pods: %v", err)
	}

	// Check if any other pod from this gateway is still running
	if slices.ContainsFunc(lst.Items, func(item unstructured.Unstructured) bool {
		return item.GetName() != pod.GetName()
	}) {
		// There are more pods from this gateway running
		return false, nil
	}

	// Get the Gateway object to extract listener info for the patch policy
	gateway, err := e.client.Resource(gatewayGVR).Namespace(gatewayNamespace).Get(context.TODO(), gatewayName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("error getting gateway %s/%s: %w", gatewayNamespace, gatewayName, err)
	}

	// Ensure the EnvoyPatchPolicy is deleted for this gateway
	if err := e.envoyGatewayInjectionPattern.Deleted(context.TODO(), gateway); err != nil {
		return false, err
	}

	return true, nil
}

func (e *envoyGatewaySidecarPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	return admissionregistrationv1.MatchCondition{
		Expression: fmt.Sprintf("'%s' in object.metadata.labels", owningGatewayNameLabel),
	}
}

func (e *envoyGatewaySidecarPattern) Added(context.Context, *unstructured.Unstructured) error {
	// Do nothing when a gateway is added, wait for its pods to flow through MutatePod
	return nil
}

// MutatePod injects the appsec processor sidecar and ensures the EnvoyPatchPolicy exists.
func (e *envoyGatewaySidecarPattern) MutatePod(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	gatewayName := pod.Labels[owningGatewayNameLabel]
	gatewayNamespace := pod.Labels[owningGatewayNamespaceLabel]

	// Get the Gateway object to extract listener info for the patch policy
	gateway, err := e.client.Resource(gatewayGVR).Namespace(gatewayNamespace).Get(context.TODO(), gatewayName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("error getting gateway %s/%s: %w", gatewayNamespace, gatewayName, err)
	}

	// Ensure the EnvoyPatchPolicy exists for this gateway
	if err := e.envoyGatewayInjectionPattern.Added(context.TODO(), gateway); err != nil {
		return false, err
	}

	// Build and inject processor container
	container := sidecar.BuildExtProcProcessorContainer(e.config.Sidecar)
	pod.Spec.Containers = append(pod.Spec.Containers, container)

	e.logger.Infof("Injected appsec processor sidecar into pod %s", mutatecommon.PodString(pod))

	return true, nil
}
