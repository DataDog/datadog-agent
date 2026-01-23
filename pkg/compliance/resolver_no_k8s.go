// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

package compliance

import (
	"context"
)

// KubernetesGroupsAndResourcesProvider is a stub function for no k8s agent (security agent)
type KubernetesGroupsAndResourcesProvider func() error

// KubernetesProvider is a stub function for no k8s agent (security agent)
type KubernetesProvider func(context.Context) error

type k8sapiserverResolver struct{}

func newK8sapiserverResolver(context.Context, ResolverOptions) *k8sapiserverResolver {
	return &k8sapiserverResolver{}
}

func (r *k8sapiserverResolver) close() {}

func (r *k8sapiserverResolver) isEnabled() bool {
	return false
}

func (r *k8sapiserverResolver) resolveKubeClusterID(context.Context) string {
	return ""
}

func (r *k8sapiserverResolver) resolveKubeApiserver(context.Context, string, InputSpecKubeapiserver) (interface{}, error) {
	return nil, ErrIncompatibleEnvironment
}
