// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package envoygateway

import (
	"context"
	"fmt"
	"path"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/sidecar"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const (
	owningGatewayNameLabel      = "gateway.envoyproxy.io/owning-gateway-name"
	owningGatewayNamespaceLabel = "gateway.envoyproxy.io/owning-gateway-namespace"
	envoyProxyContainerName     = "envoy"
)

var _ appsecconfig.SidecarInjectionPattern = (*envoyGatewaySidecarPattern)(nil)

type envoyGatewaySidecarPattern struct {
	*envoyGatewayInjectionPattern
}

func (e *envoyGatewaySidecarPattern) MatchCondition() admissionregistrationv1.MatchCondition {
	return admissionregistrationv1.MatchCondition{
		Expression: fmt.Sprintf("'%s' in object.metadata.labels", owningGatewayNameLabel),
	}
}

func (e *envoyGatewaySidecarPattern) IsNamespaceEligible(ns string) bool {
	return ns == envoyGatewaySystemNamespace
}

func (e *envoyGatewaySidecarPattern) ShouldMutatePod(pod *corev1.Pod) bool {
	if pod.Labels[owningGatewayNameLabel] == "" {
		return false
	}

	hasEnvoyContainer := false
	for _, container := range pod.Spec.Containers {
		switch container.Name {
		case envoyProxyContainerName:
			hasEnvoyContainer = true
		case sidecar.SidecarContainerName:
			e.logger.Debugf("Pod %s already has appsec UDS ext_proc sidecar", mutatecommon.PodString(pod))
			return false
		}
	}
	if !hasEnvoyContainer {
		return false
	}

	for _, volume := range pod.Spec.Volumes {
		if volume.Name == sidecar.SharedSocketVolumeName {
			e.logger.Debugf("Pod %s already has appsec UDS socket volume", mutatecommon.PodString(pod))
			return false
		}
	}

	return true
}

func (e *envoyGatewaySidecarPattern) PodDeleted(*corev1.Pod, string, dynamic.Interface) (bool, error) {
	return false, nil
}

// Added is a no-op in sidecar mode: the Backend + EnvoyExtensionPolicy are created lazily on the
// first pod mutation (see MutatePod), so Envoy Gateway is never directed at the UDS ext_proc Backend
// before at least one data-plane pod actually has the injected sidecar/socket. Teardown stays
// Gateway-informer-driven via the inherited Deleted().
func (e *envoyGatewaySidecarPattern) Added(context.Context, *unstructured.Unstructured) error {
	return nil
}

func (e *envoyGatewaySidecarPattern) MutatePod(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	for _, container := range pod.Spec.Containers {
		if container.Name == sidecar.SidecarContainerName {
			e.logger.Debugf("Pod %s already has appsec UDS ext_proc sidecar", mutatecommon.PodString(pod))
			return false, nil
		}
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == sidecar.SharedSocketVolumeName {
			e.logger.Debugf("Pod %s already has appsec UDS socket volume", mutatecommon.PodString(pod))
			return false, nil
		}
	}

	e.warnIfBackendDisabled(context.TODO(), envoyGatewaySystemNamespace)

	gwName := pod.Labels[owningGatewayNameLabel]
	if gwName == "" {
		e.logger.Warnf("Cannot resolve Envoy Gateway for pod %s: missing %q label; skipping appsec sidecar injection", mutatecommon.PodString(pod), owningGatewayNameLabel)
		return false, nil
	}
	gwNamespace := pod.Labels[owningGatewayNamespaceLabel]
	if gwNamespace == "" {
		e.logger.Warnf("Cannot resolve Envoy Gateway for pod %s: missing %q label; skipping appsec sidecar injection", mutatecommon.PodString(pod), owningGatewayNamespaceLabel)
		return false, nil
	}

	gw := &unstructured.Unstructured{}
	gw.SetName(gwName)
	gw.SetNamespace(gwNamespace)
	if err := e.envoyGatewayInjectionPattern.Added(context.TODO(), gw); err != nil {
		return false, fmt.Errorf("could not ensure envoy gateway appsec resources: %w", err)
	}

	volumeName := sidecar.EnsureSharedSocketVolume(pod)
	if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.FSGroup != nil && *pod.Spec.SecurityContext.FSGroup != e.config.Sidecar.RunAsUser {
		e.logger.Warnf("Pod %s already has fsGroup %d; leaving it unchanged instead of setting appsec sidecar fsGroup %d", mutatecommon.PodString(pod), *pod.Spec.SecurityContext.FSGroup, e.config.Sidecar.RunAsUser)
	}
	sidecar.EnsureSocketFSGroup(pod, e.config.Sidecar.RunAsUser)

	mountDir := path.Dir(e.config.Sidecar.UDSPath)
	if err := sidecar.MountSocketIntoContainer(pod, envoyProxyContainerName, volumeName, mountDir); err != nil {
		e.recorder.Eventf(
			&corev1.ObjectReference{Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, APIVersion: "v1"},
			corev1.EventTypeWarning,
			EventReasonSidecarInjectionSkipped,
			"envoy container not found, skipping appsec sidecar injection: %v",
			err,
		)
		e.logger.Warnf("Pod %s does not have envoy container, skipping appsec sidecar injection: %v", mutatecommon.PodString(pod), err)
		return false, nil
	}

	pod.Spec.Containers = append(pod.Spec.Containers, sidecar.BuildExtProcProcessorContainerUDS(e.config.Sidecar))
	e.logger.Infof("Injected appsec UDS ext_proc sidecar into pod %s", mutatecommon.PodString(pod))

	return true, nil
}
