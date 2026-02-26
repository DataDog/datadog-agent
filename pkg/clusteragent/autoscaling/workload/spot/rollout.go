// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// rollout triggers a rolling restart of a workload.
type rollout interface {
	restart(ctx context.Context, owner ownerKey, at time.Time) (bool, error)
}

type kubeRollout struct {
	client dynamic.Interface
}

func newKubeRollout(client dynamic.Interface) *kubeRollout {
	return &kubeRollout{client: client}
}

// restart patches the owning workload's pod template to trigger a rolling restart equivalent to kubectl rollout restart.
// Timestamp is written to the [SpotDisabledUntilAnnotation] annotation.
// Returns true if the patch changed the workload, false if it was already set to the same timestamp.
func (r *kubeRollout) restart(ctx context.Context, owner ownerKey, at time.Time) (bool, error) {
	gvr, err := kindGVR(owner.Kind)
	if err != nil {
		return false, err
	}

	timestamp := at.Format(time.RFC3339)

	// Check current annotation to avoid a no-op patch.
	current, err := r.client.Resource(gvr).Namespace(owner.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get %s: %w", owner, err)
	}

	var patchData []byte
	switch owner.Kind {
	case kubernetes.DeploymentKind, kubernetes.StatefulSetKind:
		existing, _, _ := unstructured.NestedString(current.Object, "spec", "template", "metadata", "annotations", SpotDisabledUntilAnnotation)
		if existing == timestamp {
			return false, nil
		}
		patch := map[string]any{
			"metadata": map[string]any{"resourceVersion": current.GetResourceVersion()},
			"spec": map[string]any{
				"template": map[string]any{
					"metadata": map[string]any{
						"annotations": map[string]string{SpotDisabledUntilAnnotation: timestamp},
					},
				},
			},
		}
		patchData, _ = json.Marshal(patch)
	default:
		return false, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}

	_, err = r.client.Resource(gvr).Namespace(owner.Namespace).Patch(ctx, owner.Name, types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		return false, err
	}
	return true, nil
}

// kindGVR returns the GroupVersionResource for a given workload kind.
func kindGVR(kind string) (schema.GroupVersionResource, error) {
	switch kind {
	case kubernetes.DeploymentKind:
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, nil
	case kubernetes.StatefulSetKind:
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, nil
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("unsupported kind %q", kind)
	}
}
