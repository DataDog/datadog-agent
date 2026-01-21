// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package istio

import (
	"context"
	"fmt"
	"sync"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/sidecar"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

const (
	gatewayClassNamePodLabel = "gateway.networking.k8s.io/gateway-class-name"
)

var (
	_               appsecconfig.SidecarInjectionPattern = (*istioGatewaySidecarPattern)(nil)
	podSelectorOnce                                      = sync.OnceValue(func() labels.Selector {
		selector := labels.NewSelector()
		reqName, _ := labels.NewRequirement(gatewayClassNamePodLabel, selection.Exists, nil)
		return selector.Add(*reqName)
	})
)

// istioGatewaySidecarPattern wraps the external pattern for SIDECAR mode
type istioGatewaySidecarPattern struct {
	*istioInjectionPattern
}

func (e *istioGatewaySidecarPattern) Added(context.Context, *unstructured.Unstructured) error {
	// Do nothing when a gateway is added, wait for its pods to flow through [InjectSidecar]
	return nil
}

// InjectSidecar wait for the first pod created by a certain gateway class to arrive to add our envoy filter to the mix.
func (e *istioGatewaySidecarPattern) InjectSidecar(ctx context.Context, pod *corev1.Pod, _ string) (bool, error) {
	// Check if sidecar already exists
	if sidecar.HasProcessorSidecar(pod) {
		e.logger.Debugf("Pod %s already has appsec processor sidecar", mutatecommon.PodString(pod))
		return false, nil
	}

	gatewayClassName := pod.Labels[gatewayClassNamePodLabel]

	gateway, err := e.client.Resource(gatewayClassGVR).Get(ctx, gatewayClassName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("error getting gatewayclass %s: %w", gatewayClassName, err)
	}

	if ok, err := isGatewayClassFromIstio(gateway); !ok || err != nil {
		return false, err // err may be nil
	}

	if err := e.istioInjectionPattern.Added(ctx, gateway); err != nil {
		return false, err
	}

	// Build and inject processor container
	container := sidecar.BuildExtProcProcessorContainer(e.config.Sidecar)
	pod.Spec.Containers = append(pod.Spec.Containers, container)

	e.logger.Infof("Injected appsec processor sidecar into pod %s", mutatecommon.PodString(pod))

	return true, nil
}

func (e *istioGatewaySidecarPattern) SidecarDeleted(context.Context, *corev1.Pod, string) error {
	// No need to do anything when a pod is deleted
	return nil
}

func (e *istioGatewaySidecarPattern) PodSelector() labels.Selector {
	return podSelectorOnce()
}
