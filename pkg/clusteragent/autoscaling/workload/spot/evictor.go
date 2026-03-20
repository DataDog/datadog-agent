// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// podEvictor evicts a pod by namespace and name.
type podEvictor interface {
	evictPod(ctx context.Context, namespace, name string) error
}

type kubePodEvictor struct {
	client kubernetes.Interface
}

func newKubePodEvictor(client kubernetes.Interface) *kubePodEvictor {
	return &kubePodEvictor{client: client}
}

// evictPod gets the pod to verify it is still pending, then evicts it.
// Returns nil if the pod is not found or is no longer pending (already resolved).
func (e *kubePodEvictor) evictPod(ctx context.Context, namespace, name string) error {
	pod, err := e.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil // already gone
	}
	if err != nil {
		return err
	}
	if pod.Status.Phase != corev1.PodPending {
		return nil // no longer pending, skip
	}
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	err = e.client.PolicyV1().Evictions(namespace).Evict(ctx, eviction)
	if errors.IsNotFound(err) {
		return nil // already gone
	}
	return err
}
