// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package evictor provides a shared API for evicting Kubernetes pods from
// the Cluster Agent. It wraps the policy/v1 Eviction subresource and handles
// PodDisruptionBudget-rejected evictions gracefully.
package evictor

import (
	"context"
	"net/http"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var podGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

// EvictResult is the outcome of an Evict call.
type EvictResult int

const (
	// Evicted means the eviction was accepted by the API server.
	Evicted EvictResult = iota
	// PDBBlocked means the eviction was rejected by a PodDisruptionBudget (HTTP 429).
	PDBBlocked
	// Skipped means this instance is not the leader; no API call was made.
	Skipped
	// Error means an error occurred while evicting the pod.
	Error
)

// Client issues policy/v1 Evictions against pods via the dynamic client.
// It optionally enforces leader election so evictions are only issued by the
// cluster agent leader.
type Client struct {
	client   dynamic.Interface
	isLeader func() bool
}

// NewClient creates a Client with the given dynamic client and an optional
// leader check function. If isLeader is non-nil, Evict returns ResultSkipped
// when the current instance is not the leader.
func NewClient(client dynamic.Interface, isLeader func() bool) *Client {
	return &Client{client: client, isLeader: isLeader}
}

// Evict creates a policy/v1 Eviction for the named pod.
//
// Returns (ResultSkipped, nil) if the current instance is not the leader.
// Returns (ResultPDBBlocked, nil) if a PodDisruptionBudget rejected the eviction (HTTP 429).
// Returns (ResultEvicted, nil) on success.
// Returns (ResultError, err) on any other error.
func (c *Client) Evict(ctx context.Context, namespace, name string) (EvictResult, error) {
	if c.isLeader != nil && !c.isLeader() {
		log.Debugf("[evictor] not leader, skipping eviction for %s/%s", namespace, name)
		return Skipped, nil
	}

	eviction := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy/v1",
			"kind":       "Eviction",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}

	_, createErr := c.client.
		Resource(podGVR).
		Namespace(namespace).
		Create(ctx, eviction, metav1.CreateOptions{}, "eviction")

	if createErr == nil {
		log.Debugf("[evictor] successfully evicted pod %s/%s", namespace, name)
		return Evicted, nil
	}

	if statusErr, ok := createErr.(*k8serrors.StatusError); ok {
		if statusErr.ErrStatus.Code == http.StatusTooManyRequests {
			log.Debugf("[evictor] eviction of pod %s/%s blocked by PodDisruptionBudget", namespace, name)
			return PDBBlocked, nil
		}
	}

	return Error, createErr
}
