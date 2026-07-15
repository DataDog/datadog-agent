// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"context"

	v1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// InjectionMode represents the deployment mode for the AppSec processor
type InjectionMode string

const (
	// InjectionModeExternal configures proxies to call an external processor service
	InjectionModeExternal InjectionMode = "external"

	// InjectionModeSidecar injects the processor as a sidecar in proxy pods
	InjectionModeSidecar InjectionMode = "sidecar"
)

// MutationOutcome names the terminal state of a MutatePod / PodDeleted call.
type MutationOutcome int

const (
	MutationMutated MutationOutcome = iota // success, err == nil
	MutationSkipped                        // owned but declined, err is *MutationSkippedReason
	MutationError                          // real failure, err is a real error
)

// MutationSkipReason is a bounded enum of owned-but-skipped reasons.
type MutationSkipReason string

const (
	SkipReasonAlreadySidecar          MutationSkipReason = "already_sidecar"
	SkipReasonAlreadySocketVolume     MutationSkipReason = "already_socket_volume"
	SkipReasonMissingUDSPath          MutationSkipReason = "missing_uds_path"
	SkipReasonGatewayOptOut           MutationSkipReason = "gateway_opt_out"
	SkipReasonAlreadyInitSidecar      MutationSkipReason = "already_init_sidecar"
	SkipReasonCrossNamespaceConfigMap MutationSkipReason = "cross_namespace_configmap"
	SkipReasonInvalidConfigMapArg     MutationSkipReason = "invalid_configmap_arg"
	SkipReasonUnknown                 MutationSkipReason = "unknown" // decorator-only sentinel; never emitted by proxies
)

type MutationSkippedReason struct {
	Reason MutationSkipReason
}

func (s *MutationSkippedReason) Error() string { return "mutation skipped: " + string(s.Reason) }

// InjectionPattern is the main interface to implement to support a new proxy type
// for appsec injection. It is similar to the controller pattern used in
// controller-runtime, but much simpler.
// The controller watches for changes to a specific resource (Resource method)
// and calls the Added, Modified, Deleted methods accordingly.
// The IsInjectionPossible method is called at startup to determine if the pattern
// can be used in the current cluster (e.g. if the required CRDs are present).
// The Namespace method returns the namespace to watch, or metav1.NamespaceAll to watch all namespaces.
// The methods are called sequentially, there is no need to handle concurrency.
// The methods should return an error if something goes wrong, in which case the
// object will be re-queued with a backoff.
type InjectionPattern interface {
	// Mode returns the injection mode (EXTERNAL or SIDECAR)
	Mode() InjectionMode
	// IsInjectionPossible returns true if the pattern can be used in the current cluster.
	IsInjectionPossible(ctx context.Context) error
	// Resource returns the GroupVersionResource to watch.
	Resource() schema.GroupVersionResource
	// Namespace returns the namespace to watch, or metav1.NamespaceAll to watch all namespaces.
	Namespace() string
	// Added is called when an object is added or at startup for existing objects. It should be idempotent.
	Added(ctx context.Context, obj *unstructured.Unstructured) error
	// Deleted is called when an object is deleted. It should be idempotent.
	Deleted(ctx context.Context, obj *unstructured.Unstructured) error
}

// Starter optional reconciler (e.g. nginx ConfigMap sync).
// Patterns that need background reconciliation implement this interface.
type Starter interface {
	Start(ctx context.Context) error
}

// SidecarInjectionPattern extends InjectionPattern for SIDECAR mode
// Implementations provide both proxy configuration AND sidecar injection logic
type SidecarInjectionPattern interface {
	InjectionPattern

	// IsPodEligible is a PURE OWNERSHIP PREDICATE. It answers "is this pod one THIS
	// pattern owns?" — nothing about idempotency, opt-out, or config validity.
	// Ownership-negative outcomes (return false) are NOT skips and are NOT counted.
	// Owned-but-skipped checks live inside MutatePod as MutationSkipped +
	// *MutationSkippedReason.
	IsPodEligible(pod *corev1.Pod, ns string) bool

	// MutatePod mutates the pod or reports why it declined.
	MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (MutationOutcome, error)

	// PodDeleted mirrors MutatePod for DELETE admissions.
	PodDeleted(pod *corev1.Pod, ns string, dc dynamic.Interface) (MutationOutcome, error)

	// MatchCondition is unchanged — apiserver-level prefilter, OR-ed across patterns.
	MatchCondition() v1.MatchCondition
}
