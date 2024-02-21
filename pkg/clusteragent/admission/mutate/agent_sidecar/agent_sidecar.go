// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package agentsidecar defines the mutation logic for the agentsidecar webhook
package agentsidecar

import (
	"errors"
	dca_ac "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
)

// InjectAgentSidecar handles mutating pod requests for the agentsidecat webhook
func InjectAgentSidecar(rawPod []byte, _ string, ns string, _ *authenticationv1.UserInfo, dc dynamic.Interface, _ k8s.Interface) ([]byte, error) {
	return dca_ac.Mutate(rawPod, ns, injectAgentSidecar, dc)
}

func injectAgentSidecar(pod *corev1.Pod, _ string, _ dynamic.Interface) error {
	if pod == nil {
		return errors.New("can't inject agent sidecar into nil pod")
	}

	for _, container := range pod.Spec.Containers {
		if container.Name == agentSidecarContainerName {
			log.Info("skipping agent sidecar injection: agent sidecar already exists")
			return nil
		}
	}

	agentSidecarContainer := getDefaultSidecarTemplate()

	applyProviderOverrides(agentSidecarContainer)

	pod.Spec.Containers = append(pod.Spec.Containers, *agentSidecarContainer)
	return nil
}
