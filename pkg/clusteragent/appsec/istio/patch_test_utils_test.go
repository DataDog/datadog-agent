// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package istio

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

func mutatePatternForTest(pattern appsecconfig.SidecarInjectionPattern, pod *corev1.Pod, ns string, dc dynamic.Interface) (appsecconfig.MutationOutcome, error) {
	var outcome appsecconfig.MutationOutcome
	_, err := patchtest.Run(pod, ns, dc, func(s *patch.PodSession, ns string, dc dynamic.Interface) (bool, error) {
		var err error
		outcome, err = pattern.PlanPod(s, ns, dc)
		return outcome == appsecconfig.MutationMutated, err
	})
	return outcome, err
}
