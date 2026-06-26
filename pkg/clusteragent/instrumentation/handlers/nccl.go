// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"fmt"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	ncclReadyConditionType = "NCCLProfilerReady"

	reasonNCCLConfigured = "Configured"
	reasonNCCLDeleted    = "Deleted"
	reasonNCCLInvalidEnv = "InvalidEnv"

	// Target kinds beyond the standard workload kinds in pkg/util/kubernetes.
	rayClusterKind = "RayCluster" // KubeRay
	pyTorchJobKind = "PyTorchJob" // Kubeflow training-operator
	// NamespaceKind is the namespace-scope target kind, shared with the ncclprofiler
	// webhook's namespace fallback. Exported so the two stay in sync.
	NamespaceKind = "Namespace"
	namespaceKind = NamespaceKind
)

// NCCLHandler translates DatadogInstrumentation ncclProfiler sections into
// ncclprofiler admission-webhook configuration via the shared NCCLProfilerStore.
// Independent of the APM handler (own store, own section).
type NCCLHandler struct {
	store *NCCLProfilerStore
}

// NewNCCLHandler returns the NCCL profiler DatadogInstrumentation handler.
func NewNCCLHandler(deps *Deps) *NCCLHandler {
	return &NCCLHandler{store: deps.NCCLProfilerStore}
}

// Store returns the shared NCCL profiler store the handler writes to, so the
// ncclprofiler admission webhook can read it (same instance) to decide injection.
func (h *NCCLHandler) Store() *NCCLProfilerStore {
	return h.store
}

// Name returns the unique handler name.
func (h *NCCLHandler) Name() string {
	return "nccl-profiler"
}

// HasSection reports whether the CR contains an NCCL profiler section.
func (h *NCCLHandler) HasSection(cr *datadoghq.DatadogInstrumentation) bool {
	return cr != nil && cr.Spec.Config.NCCLProfiler != nil
}

// SupportsTarget returns whether the NCCL profiler supports the target kind.
// Enforced at CR-create time by the DDI validation webhook, so every kind the injection
// webhook resolves a pod to (workloadTargetsFromPod) must appear here. Only kinds whose
// pods carry the controller as their *direct* owner reference are matchable; indirectly
// owned workloads (Deployment via ReplicaSet, CronJob via Job) are not — use Namespace.
func (h *NCCLHandler) SupportsTarget(ref autoscalingv2.CrossVersionObjectReference) bool {
	switch ref.Kind {
	case kubernetes.StatefulSetKind, kubernetes.DaemonSetKind, kubernetes.JobKind:
		return true
	case rayClusterKind, pyTorchJobKind, namespaceKind:
		return true
	default:
		return false
	}
}

// Validate checks spec.config.ncclProfiler fields during admission.
func (h *NCCLHandler) Validate(cr *datadoghq.DatadogInstrumentation) []instrumentation.ValidationError {
	if cr == nil || cr.Spec.Config.NCCLProfiler == nil {
		return nil
	}
	var errs []instrumentation.ValidationError
	for i, e := range cr.Spec.Config.NCCLProfiler.Env {
		if e.Name == "" {
			errs = append(errs, instrumentation.ValidationError{
				Type:        ncclReadyConditionType,
				Reason:      reasonNCCLInvalidEnv,
				Message:     "env var name must not be empty",
				Field:       fmt.Sprintf("spec.config.ncclProfiler.env[%d].name", i),
				HandlerName: h.Name(),
			})
		}
	}
	return errs
}

// Handle applies or removes NCCL profiler config in the shared store. Idempotent.
// New pods are injected by the ncclprofiler webhook reading the store on admission;
// existing pods inject on restart (Ray/PyTorchJob are (re)launched per run).
func (h *NCCLHandler) Handle(_ context.Context, event instrumentation.EventType, cr *datadoghq.DatadogInstrumentation) (instrumentation.HandlerStatus, error) {
	if cr == nil {
		return instrumentation.HandlerStatus{
			Type:    ncclReadyConditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  "MissingResource",
			Message: "DatadogInstrumentation resource is nil",
		}, nil
	}

	crRef := types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}

	if event == instrumentation.EventDelete {
		h.store.DeleteByCR(crRef)
		return instrumentation.HandlerStatus{
			Type:    ncclReadyConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  reasonNCCLDeleted,
			Message: fmt.Sprintf("NCCL profiler config removed for %s/%s", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
		}, nil
	}

	target := NCCLProfilerTarget{Kind: cr.Spec.TargetRef.Kind, Namespace: cr.Namespace, Name: cr.Spec.TargetRef.Name}
	if target.Kind == NamespaceKind {
		// Key by the CR's own namespace so the webhook's namespace fallback matches
		// regardless of the targetRef.Name the user supplied.
		target.Name = cr.Namespace
	}
	h.store.Upsert(target, ncclProfilerConfigFromCR(crRef, cr.Spec.Config.NCCLProfiler))

	return instrumentation.HandlerStatus{
		Type:    ncclReadyConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  reasonNCCLConfigured,
		Message: fmt.Sprintf("NCCL profiler configured for %s/%s; applies to new pods (restart existing to inject)", cr.Spec.TargetRef.Kind, cr.Spec.TargetRef.Name),
	}, nil
}

func ncclProfilerConfigFromCR(crRef types.NamespacedName, n *datadoghq.DatadogInstrumentationNCCLConfig) NCCLProfilerConfig {
	return NCCLProfilerConfig{
		CR:            crRef,
		Enabled:       n.Enabled,
		InjectorImage: n.InjectorImage,
		Env:           n.Env,
	}
}
