// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

func (w *Webhook) callPlanPatternForTest(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, appsecconfig.ProxyType, appsecconfig.MutationOutcome, error) {
	var matched bool
	var proxy appsecconfig.ProxyType
	var outcome appsecconfig.MutationOutcome
	_, err := patchtest.Run(pod, ns, dc, func(s *patch.PodSession, ns string, dc dynamic.Interface) (bool, error) {
		var err error
		matched, proxy, outcome, err = w.callPlanPattern(s, ns, dc)
		return outcome == appsecconfig.MutationMutated, err
	})
	return matched, proxy, outcome, err
}
