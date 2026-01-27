// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package istio

import (
	"context"
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

const (
	gatewayClassNamePodLabel = "gateway.networking.k8s.io/gateway-class-name"
)

var _ appsecconfig.SidecarInjectionPattern = (*istioGatewaySidecarPattern)(nil)

// istioGatewaySidecarPattern wraps the external pattern for SIDECAR mode
type istioGatewaySidecarPattern struct {
	*istioInjectionPattern
}

func (e *istioGatewaySidecarPattern) ShouldMutatePod(pod *corev1.Pod) bool {
	// Check if sidecar already exists
	if sidecar.HasProcessorSidecar(pod) {
		e.logger.Debugf("Pod %s already has appsec processor sidecar", mutatecommon.PodString(pod))
		return false
	}

	gatewayClassName := pod.Labels[gatewayClassNamePodLabel]

	gateway, err := e.client.Resource(gatewayClassGVR).Get(context.TODO(), gatewayClassName, metav1.GetOptions{})
	if err != nil {
		e.logger.Warnf("error getting gatewayclass %s: %v", gatewayClassName, err)
		return false
	}

	if ok, err := isGatewayClassFromIstio(gateway); !ok || err != nil {
		e.logger.Warnf("error parsing gatewayclass %s: %v", gatewayClassName, err)
		return false
	}

	return true
}

func (e *istioGatewaySidecarPattern) IsNamespaceEligible(string) bool {
	// We want to inject sidecar in all namespaces
	return true
}

func (e *istioGatewaySidecarPattern) PodDeleted(*corev1.Pod, string, dynamic.Interface) (bool, error) {
	// We don't care about a pod being deleted here
	return false, nil
}

func (e *istioGatewaySidecarPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	return admissionregistrationv1.MatchCondition{
		Expression: fmt.Sprintf("'%s' in object.metadata.labels", gatewayClassNamePodLabel),
	}
}

func (e *istioGatewaySidecarPattern) Added(context.Context, *unstructured.Unstructured) error {
	// Do nothing when a gateway is added, wait for its pods to flow through [InjectSidecar]
	return nil
}

// MutatePod wait for the first pod created by a certain gateway class to arrive to add our envoy filter to the mix.
func (e *istioGatewaySidecarPattern) MutatePod(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	gatewayClassName := pod.Labels[gatewayClassNamePodLabel]

	gateway, err := e.client.Resource(gatewayClassGVR).Get(context.TODO(), gatewayClassName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("error getting gatewayclass %s: %w", gatewayClassName, err)
	}
	if err := e.istioInjectionPattern.Added(context.TODO(), gateway); err != nil {
		return false, err
	}

	// Build and inject processor container
	container := sidecar.BuildExtProcProcessorContainer(e.config.Sidecar)
	pod.Spec.Containers = append(pod.Spec.Containers, container)

	e.logger.Infof("Injected appsec processor sidecar into pod %s", mutatecommon.PodString(pod))

	return true, nil
}
