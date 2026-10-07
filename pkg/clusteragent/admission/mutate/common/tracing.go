// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"fmt"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

// TraceStage measures a bounded admission processing stage. Span creation is a
// no-op when Cluster Agent tracing is disabled. Panics are recorded and rethrown.
func TraceStage(ctx context.Context, stage string, f func(context.Context) error) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "cluster_agent.admission."+stage)
	defer func() {
		if p := recover(); p != nil {
			span.Finish(tracer.WithError(fmt.Errorf("admission stage panic: %v", p)))
			panic(p)
		}
		span.Finish(tracer.WithError(err))
	}()
	return f(ctx)
}

// MutatePodWithContext uses contextual mutation when supported, falling back to
// the existing Mutator interface for mutators without internal tracing.
func MutatePodWithContext(ctx context.Context, m Mutator, pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	if contextual, ok := m.(interface {
		MutatePodWithContext(context.Context, *corev1.Pod, string, dynamic.Interface) (bool, error)
	}); ok {
		return contextual.MutatePodWithContext(ctx, pod, ns, dc)
	}
	return m.MutatePod(pod, ns, dc)
}
