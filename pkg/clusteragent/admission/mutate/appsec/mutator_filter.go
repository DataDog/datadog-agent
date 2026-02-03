// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	corev1 "k8s.io/api/core/v1"
)

var _ mutatecommon.MutationFilter = (*mutationFilter)(nil)

// mutationFilter is a minimal version of the default mutation filter in [mutatecommon.DefaultFilter] because we
// need to be able to inject in the kube-system namespace as proxy may (and by default) will be there.
type mutationFilter struct {
	ddNamespace string
}

func (m mutationFilter) ShouldMutatePod(_ *corev1.Pod) bool {
	return true
}

func (m mutationFilter) IsNamespaceEligible(ns string) bool {
	return ns != m.ddNamespace
}

func newMutationFilter() *mutationFilter {
	return &mutationFilter{
		ddNamespace: namespace.GetResourcesNamespace(),
	}
}
