// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"encoding/json"
	"fmt"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	scaleclient "k8s.io/client-go/scale"
)

// datadogClusterAgentFieldManager is the field-manager name Kubernetes
// records for writes the cluster agent issues against scaleable workloads.
// It is derived from the binary's user-agent. See the support case context
// in the PR description for how stale entries with this manager surface as
// SSA conflicts for users.
const datadogClusterAgentFieldManager = "datadog-cluster-agent"

type scaler interface {
	get(ctx context.Context, namespace, name string, gvk schema.GroupVersionKind) (*autoscalingv1.Scale, schema.GroupResource, error)
	update(ctx context.Context, gr schema.GroupResource, scale *autoscalingv1.Scale) (*autoscalingv1.Scale, error)
	// releaseReplicasOwnership removes the cluster agent's managed-fields
	// entry for the scale subresource on the target workload, so that
	// server-side appliers (e.g. Helm SSA) can write `.spec.replicas`
	// without conflicting with a stale entry left behind once the
	// DatadogPodAutoscaler stops scaling the workload.
	//
	// Safe to call when no such entry exists (returns nil).
	releaseReplicasOwnership(ctx context.Context, namespace, name string, gvk schema.GroupVersionKind) error
}

type scalerImpl struct {
	restMapper    apimeta.RESTMapper
	scaleGetter   scaleclient.ScalesGetter
	dynamicClient dynamic.Interface
}

func newScaler(restMapper apimeta.RESTMapper, scaleGetter scaleclient.ScalesGetter, dynamicClient dynamic.Interface) scaler {
	return &scalerImpl{
		restMapper:    restMapper,
		scaleGetter:   scaleGetter,
		dynamicClient: dynamicClient,
	}
}

func (sg *scalerImpl) get(ctx context.Context, namespace, name string, gvk schema.GroupVersionKind) (*autoscalingv1.Scale, schema.GroupResource, error) {
	mappings, err := sg.restMapper.RESTMappings(gvk.GroupKind())
	if err != nil {
		return nil, schema.GroupResource{}, fmt.Errorf("failed to get REST mappings for GVK: %s", gvk)
	}

	var firstErr error
	for i, mapping := range mappings {
		targetGR := mapping.Resource.GroupResource()
		scale, err := sg.scaleGetter.Scales(namespace).Get(ctx, targetGR, name, metav1.GetOptions{})
		if err == nil {
			return scale, targetGR, nil
		}

		// if this is the first error, remember it,
		// then go on and try other mappings until we find a good one
		if i == 0 {
			firstErr = err
		}
	}

	// make sure we handle an empty set of mappings
	if firstErr == nil {
		firstErr = fmt.Errorf("unrecognized resource: %s", gvk)
	}

	return nil, schema.GroupResource{}, firstErr
}

func (sg *scalerImpl) update(ctx context.Context, gr schema.GroupResource, scale *autoscalingv1.Scale) (*autoscalingv1.Scale, error) {
	return sg.scaleGetter.Scales(scale.Namespace).Update(ctx, gr, scale, metav1.UpdateOptions{})
}

func (sg *scalerImpl) releaseReplicasOwnership(ctx context.Context, namespace, name string, gvk schema.GroupVersionKind) error {
	mappings, err := sg.restMapper.RESTMappings(gvk.GroupKind())
	if err != nil {
		return fmt.Errorf("failed to get REST mappings for GVK: %s", gvk)
	}

	var firstErr error
	for i, mapping := range mappings {
		gvr := mapping.Resource
		err := sg.releaseReplicasOwnershipForGVR(ctx, namespace, name, gvr)
		if err == nil {
			return nil
		}
		if k8serrors.IsNotFound(err) {
			// Target workload no longer exists — nothing left to release.
			return nil
		}
		if i == 0 {
			firstErr = err
		}
	}

	if firstErr == nil {
		return fmt.Errorf("unrecognized resource: %s", gvk)
	}
	return firstErr
}

func (sg *scalerImpl) releaseReplicasOwnershipForGVR(ctx context.Context, namespace, name string, gvr schema.GroupVersionResource) error {
	obj, err := sg.dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Collect indices of managedFields entries owned by the cluster agent
	// on the scale subresource. Iterate in descending order so each remove
	// op leaves earlier indices stable.
	managedFields := obj.GetManagedFields()
	var indices []int
	for i, mf := range managedFields {
		if mf.Manager == datadogClusterAgentFieldManager && mf.Subresource == "scale" {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return nil
	}

	patch := make([]map[string]string, 0, len(indices))
	for j := len(indices) - 1; j >= 0; j-- {
		patch = append(patch, map[string]string{
			"op":   "remove",
			"path": fmt.Sprintf("/metadata/managedFields/%d", indices[j]),
		})
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal managedFields patch: %w", err)
	}

	_, err = sg.dynamicClient.Resource(gvr).Namespace(namespace).Patch(ctx, name, types.JSONPatchType, body, metav1.PatchOptions{})
	return err
}
