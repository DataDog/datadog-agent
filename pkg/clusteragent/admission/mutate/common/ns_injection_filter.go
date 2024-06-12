// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	corev1 "k8s.io/api/core/v1"
)

type InjectionFilter struct {
	NSFilter NamespaceInjectionFilter
}

func (f InjectionFilter) ShouldMutatePod(pod *corev1.Pod) bool {
	return ShouldMutatePod(pod, f.NSFilter)
}

// NamespaceInjectionFilter represents a contract to be able to filter out which pods are
// eligible for mutation/injection.
//
// See [autoinstrumentation.GetInjectionFilter].
type NamespaceInjectionFilter interface {
	IsNamespaceEligible(ns string) bool
	Err() error
}
