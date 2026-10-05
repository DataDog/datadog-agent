// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package cwsinstrumentation implements the webhook that injects CWS pod and
// pod exec instrumentation
package cwsinstrumentation

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

func (ci *CWSInstrumentation) injectCWSPodInstrumentation(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	if pod == nil {
		return ci.planCWSPodInstrumentation(nil, ns, dc)
	}
	return patchtest.Run(pod, ns, dc, ci.planCWSPodInstrumentation)
}
