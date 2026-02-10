// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
)

// NoopProvider is a provider that intentionally performs no injection.
// It is used when injection is requested but cannot be performed (e.g. incompatible cluster/runtime).
type NoopProvider struct {
	err error
}

func newNoopProvider(err error) *NoopProvider {
	if err == nil {
		err = errors.New("injection disabled")
	}
	return &NoopProvider{err: err}
}

// Err returns the skip reason that will be reported in MutationResult.Err.
func (p *NoopProvider) Err() error { return p.err }

func (p *NoopProvider) InjectInjector(_ *corev1.Pod, _ InjectorConfig) MutationResult {
	return MutationResult{
		Status: MutationStatusSkipped,
		Err:    p.err,
	}
}

func (p *NoopProvider) InjectLibrary(_ *corev1.Pod, _ LibraryConfig) MutationResult {
	return MutationResult{
		Status: MutationStatusSkipped,
		Err:    p.err,
	}
}

var _ LibraryInjectionProvider = (*NoopProvider)(nil)
