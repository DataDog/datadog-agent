// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && kubeapiserver

package autoinstrumentation

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

// WithResetInjectionFilter is useful for testing when you want to make sure
// that you're getting a fresh instance based on configuration changes.
//
// The injection filter is reset before and after `do` runs.
func WithResetInjectionFilter[T any](do func(f common.InjectionFilter) T) T {
	resetInjectionFilter()
	defer resetInjectionFilter()
	return do(common.InjectionFilter{NSFilter: GetInjectionFilter()})
}

// WithResetInjectionFilter1 reflects the structure of NewWebhook constructors (2 arguments),
// that depend on workloadmeta.Component, but this is generic instead of depending on it directly.
func WithResetInjectionFilter1[C, T any](c C, do func(C, common.InjectionFilter) T) T {
	return WithResetInjectionFilter(func(f common.InjectionFilter) T { return do(c, f) })
}

// WithResetInjectionFilter2 reflects the structure of constructors (3 arguments),
// that depend on workloadmeta.Component, but this is generic instead of depending on it directly.
func WithResetInjectionFilter2[C1, C2, T any](c1 C1, c2 C2, do func(C1, C2, common.InjectionFilter) T) T {
	return WithResetInjectionFilter(func(f common.InjectionFilter) T { return do(c1, c2, f) })
}

// resetInjectionFilter resets the singleton autoInstrumentationFilter.
func resetInjectionFilter() {
	autoInstrumentationFilter = &injectionFilter{}
}
