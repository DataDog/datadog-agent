// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"errors"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

// Do not attach the original error: mutation errors can contain pod data,
// annotation values, or Kubernetes object identifiers.
var errMutation = errors.New("admission mutation failed")

// StartMutationSpan starts an internal admission span. Stage must be a fixed
// operation name, never a value derived from the admitted object.
func StartMutationSpan(ctx context.Context, stage string) (*tracer.Span, context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	return tracer.StartSpanFromContext(ctx, "cluster_agent.admission."+stage)
}

// FinishMutationSpan records failure without exporting the original error.
func FinishMutationSpan(span *tracer.Span, err error) {
	if err != nil {
		span.Finish(tracer.WithError(errMutation))
		return
	}
	span.Finish()
}

// ContextMutatorFunc is a mutation callback that preserves the request context.
type ContextMutatorFunc func(context.Context, *corev1.Pod, string, dynamic.Interface) (bool, error)

// ContextMutator extends Mutator for stages with child spans. Keeping the base
// interface allows unrelated mutators to continue to use the existing API.
type ContextMutator interface {
	MutatePodWithContext(context.Context, *corev1.Pod, string, dynamic.Interface) (bool, error)
}

// WithContext adapts a mutator, preserving context when the mutator supports it.
func WithContext(m Mutator) ContextMutatorFunc {
	if contextual, ok := m.(ContextMutator); ok {
		return contextual.MutatePodWithContext
	}
	return func(_ context.Context, pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
		return m.MutatePod(pod, ns, dc)
	}
}

// ShouldMutatePodWithContext preserves context through filters which perform
// traced selection, without changing the contract of simple mutation filters.
func ShouldMutatePodWithContext(ctx context.Context, filter MutationFilter, pod *corev1.Pod) bool {
	if contextual, ok := filter.(interface {
		ShouldMutatePodWithContext(context.Context, *corev1.Pod) bool
	}); ok {
		return contextual.ShouldMutatePodWithContext(ctx, pod)
	}
	return filter.ShouldMutatePod(pod)
}
