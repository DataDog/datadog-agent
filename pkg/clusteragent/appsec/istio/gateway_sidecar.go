// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package istio

import (
	"context"
	"errors"
	"fmt"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/sidecar"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

var _ appsecconfig.SidecarInjectionPattern = (*istioNativeGatewaySidecarPattern)(nil)

// istioNativeGatewaySidecarPattern wraps the native gateway pattern for SIDECAR mode
type istioNativeGatewaySidecarPattern struct {
	*istioNativeGatewayPattern
}

func (e *istioNativeGatewaySidecarPattern) IsPodEligible(pod *corev1.Pod, _ string) bool {
	// List all Istio native Gateways and check if any selector matches the pod's labels
	list, err := e.client.Resource(istioGatewayGVR).Namespace(corev1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		e.logger.Warnf("error listing Istio gateways: %v", err)
		return false
	}

	for i := range list.Items {
		if selectorMatchesPod(&list.Items[i], pod) {
			return true
		}
	}

	return false
}

func (e *istioNativeGatewaySidecarPattern) PodDeleted(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	// PodDeleted is a no-op; the returned outcome is only consulted for the DELETE admission error path (the metric is not emitted on delete).
	return appsecconfig.MutationMutated, nil
}

func (e *istioNativeGatewaySidecarPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	// Standard Istio gateway pods (istio-ingressgateway, istio-egressgateway) always carry
	// the "istio" label key (e.g. istio=ingressgateway). This is a broad pre-filter that passes
	// gateway proxy pods through to IsPodEligible, which performs precise spec.selector matching.
	return admissionregistrationv1.MatchCondition{
		Expression: "'istio' in object.metadata.labels",
	}
}

func (e *istioNativeGatewaySidecarPattern) Added(context.Context, *unstructured.Unstructured) error {
	// No-op: EnvoyFilter creation is lazy, happening on first pod mutation
	return nil
}

// MutatePod creates the EnvoyFilter lazily on first pod mutation and injects the sidecar container
func (e *istioNativeGatewaySidecarPattern) MutatePod(pod *corev1.Pod, _ string, _ dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	if sidecar.HasProcessorSidecar(pod) {
		return appsecconfig.MutationSkipped, &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonAlreadySidecar}
	}

	// Verify at least one Istio Gateway's selector matches this pod
	list, err := e.client.Resource(istioGatewayGVR).Namespace(corev1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return appsecconfig.MutationError, fmt.Errorf("error listing Istio gateways: %w", err)
	}

	matched := false
	for i := range list.Items {
		if selectorMatchesPod(&list.Items[i], pod) {
			matched = true
			break
		}
	}

	if !matched {
		return appsecconfig.MutationError, errors.New("no Istio gateway selector matched pod after eligibility check")
	}

	// Lazy EnvoyFilter creation (idempotent)
	if err := e.createEnvoyFilter(context.TODO(), e.config.IstioNamespace); err != nil {
		return appsecconfig.MutationError, fmt.Errorf("could not create Envoy Filter: %w", err)
	}

	// Build and inject processor container
	container := sidecar.BuildExtProcProcessorContainer(e.config.Sidecar)
	pod.Spec.Containers = append(pod.Spec.Containers, container)

	e.logger.Infof("Injected appsec processor sidecar into pod %s", mutatecommon.PodString(pod))

	return appsecconfig.MutationMutated, nil
}

// selectorMatchesPod checks if a networking.istio.io/v1 Gateway's spec.selector matches a pod's labels
func selectorMatchesPod(gateway *unstructured.Unstructured, pod *corev1.Pod) bool {
	selector, found, err := unstructured.NestedStringMap(gateway.UnstructuredContent(), "spec", "selector")
	if err != nil || !found || len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if pod.Labels[k] != v {
			return false
		}
	}
	return true
}
