// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package ncclprofiler

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

func mutatePod(pod *corev1.Pod, injectorImage, hostSocketDir, clientSocketDir, socketFilename string, initResources *corev1.ResourceRequirements) (bool, error) {
	return patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return planPod(s, injectorImage, hostSocketDir, clientSocketDir, socketFilename, initResources)
	})
}
