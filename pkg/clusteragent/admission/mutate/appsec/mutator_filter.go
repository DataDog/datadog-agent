// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	corev1 "k8s.io/api/core/v1"
)

var _ mutatecommon.MutationFilter = (*mutationFilter)(nil)

// mutationFilter is a minimal MutationFilter that allows mutation in every
// namespace, including kube-system where proxies may run by default.
type mutationFilter struct{}

func (m mutationFilter) ShouldMutatePod(_ *corev1.Pod) bool {
	return true
}

func newMutationFilter() *mutationFilter {
	return &mutationFilter{}
}
