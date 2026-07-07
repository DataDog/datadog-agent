// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package envoygateway

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/sidecar"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const (
	owningGatewayNameLabel      = "gateway.envoyproxy.io/owning-gateway-name"
	owningGatewayNamespaceLabel = "gateway.envoyproxy.io/owning-gateway-namespace"
	envoyProxyContainerName     = "envoy"
	appsecEnabledLabel          = "appsec.datadoghq.com/enabled"
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

// envoyGatewayNamespace returns the configured Envoy Gateway data-plane namespace, falling back to
// the envoy-gateway-system default when unset (e.g. in tests that build Config directly).
func (e *envoyGatewayInjectionPattern) envoyGatewayNamespace() string {
	if ns := e.config.EnvoyGatewayNamespace; ns != "" {
		return ns
	}
	return envoyGatewaySystemNamespace
}

// envoyGatewayControllerNamespace returns the configured Envoy Gateway control-plane namespace, where
// the envoy-gateway-config ConfigMap lives, falling back to envoy-gateway-system when unset. It is
// resolved separately from the data-plane namespace because proxies can run in Gateway namespaces
// while the controller config stays in the control-plane namespace.
func (e *envoyGatewayInjectionPattern) envoyGatewayControllerNamespace() string {
	if ns := e.config.EnvoyGatewayControllerNamespace; ns != "" {
		return ns
	}
	return envoyGatewaySystemNamespace
}

func (e *envoyGatewaySidecarPattern) IsPodEligible(pod *corev1.Pod, ns string) bool {
	if ns != e.envoyGatewayNamespace() {
		return false
	}
	if pod.Labels[owningGatewayNameLabel] == "" {
		return false
	}
	if pod.Labels[owningGatewayNamespaceLabel] == "" {
		return false
	}

	for _, container := range pod.Spec.Containers {
		if container.Name == envoyProxyContainerName {
			return true
		}
	}
	return false
}

func (e *envoyGatewaySidecarPattern) PodDeleted(*corev1.Pod, string, dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	// PodDeleted is a no-op; the returned outcome is only consulted for the DELETE admission error path (the metric is not emitted on delete).
	return appsecconfig.MutationMutated, nil
}

// Added is a no-op in sidecar mode: the Backend + EnvoyExtensionPolicy are created lazily on the
// first pod mutation (see MutatePod), so Envoy Gateway is never directed at the UDS ext_proc Backend
// before at least one data-plane pod actually has the injected sidecar/socket. Teardown stays
// Gateway-informer-driven via the inherited Deleted().
func (e *envoyGatewaySidecarPattern) Added(context.Context, *unstructured.Unstructured) error {
	return nil
}

func (e *envoyGatewaySidecarPattern) MutatePod(pod *corev1.Pod, _ string, _ dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	for _, container := range pod.Spec.Containers {
		if container.Name == sidecar.SidecarContainerName {
			return appsecconfig.MutationSkipped, &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonAlreadySidecar}
		}
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == sidecar.SharedSocketVolumeName {
			return appsecconfig.MutationSkipped, &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonAlreadySocketVolume}
		}
	}

	if strings.TrimSpace(e.config.Sidecar.UDSPath) == "" {
		return appsecconfig.MutationSkipped, &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonMissingUDSPath}
	}

	e.warnIfBackendDisabled(context.TODO(), e.envoyGatewayControllerNamespace())

	gwName := pod.Labels[owningGatewayNameLabel]
	if gwName == "" {
		e.logger.Warnf("Cannot resolve Envoy Gateway for pod %s: missing %q label; failed to inject appsec sidecar", mutatecommon.PodString(pod), owningGatewayNameLabel)
		return appsecconfig.MutationError, errors.New("owning gateway name label missing after eligibility check")
	}
	gwNamespace := pod.Labels[owningGatewayNamespaceLabel]
	if gwNamespace == "" {
		e.logger.Warnf("Cannot resolve Envoy Gateway for pod %s: missing %q label; failed to inject appsec sidecar", mutatecommon.PodString(pod), owningGatewayNamespaceLabel)
		return appsecconfig.MutationError, errors.New("owning gateway namespace label missing after eligibility check")
	}

	// The Gateway informer honors the appsec.datadoghq.com/enabled=false opt-out, but in sidecar
	// mode resources are created here (Added is a no-op), so re-check the owning Gateway's opt-out
	// label before mutating. Fail open: if the Gateway cannot be read, proceed with injection.
	if gw, err := e.client.Resource(gatewayGVR).Namespace(gwNamespace).Get(context.TODO(), gwName, metav1.GetOptions{}); err == nil {
		if gw.GetLabels()[appsecEnabledLabel] == "false" {
			return appsecconfig.MutationSkipped, &appsecconfig.MutationSkippedReason{Reason: appsecconfig.SkipReasonGatewayOptOut}
		}
	} else if !k8serrors.IsNotFound(err) {
		e.logger.Warnf("Could not read Envoy Gateway %s/%s to check appsec opt-out, proceeding with injection: %v", gwNamespace, gwName, err)
	}

	gw := &unstructured.Unstructured{}
	gw.SetName(gwName)
	gw.SetNamespace(gwNamespace)
	if err := e.envoyGatewayInjectionPattern.Added(context.TODO(), gw); err != nil {
		return appsecconfig.MutationError, fmt.Errorf("could not ensure envoy gateway appsec resources: %w", err)
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
			"envoy container not found, failed to inject appsec sidecar: %v",
			err,
		)
		e.logger.Warnf("Pod %s does not have envoy container, failed to inject appsec sidecar: %v", mutatecommon.PodString(pod), err)
		return appsecconfig.MutationError, fmt.Errorf("failed to mount appsec socket into envoy container for pod %s: %w", mutatecommon.PodString(pod), err)
	}

	pod.Spec.Containers = append(pod.Spec.Containers, sidecar.BuildExtProcProcessorContainerUDS(e.config.Sidecar))
	e.logger.Infof("Injected appsec UDS ext_proc sidecar into pod %s", mutatecommon.PodString(pod))

	return appsecconfig.MutationMutated, nil
}
