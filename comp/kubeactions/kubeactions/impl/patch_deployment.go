// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
)

// PatchDeploymentExecutor executes patch_deployment actions.
type PatchDeploymentExecutor struct {
	clientset kubernetes.Interface
}

// NewPatchDeploymentExecutor creates a new PatchDeploymentExecutor.
func NewPatchDeploymentExecutor(clientset kubernetes.Interface) *PatchDeploymentExecutor {
	return &PatchDeploymentExecutor{clientset: clientset}
}

// resolvePatchType maps a patch strategy string to the corresponding Kubernetes
// patch type. Defaults to strategic merge if unspecified or unrecognized.
func resolvePatchType(strategy string) types.PatchType {
	switch strategy {
	case "merge":
		return types.MergePatchType
	case "json":
		return types.JSONPatchType
	default:
		return types.StrategicMergePatchType
	}
}

// applyUIDGuard rewrites patch so it fails atomically when the live object's UID
// no longer matches uid, closing the check-then-patch race: the pre-Get UID
// comparison can go stale if the object is deleted and recreated under the same
// name between the Get and the Patch. PatchOptions carries no UID precondition,
// so the guard is embedded in the patch body itself:
//   - JSON patch (RFC 6902): prepend a `test` op on /metadata/uid; a mismatch
//     makes the API server reject the whole patch.
//   - strategic / merge patch: set metadata.uid — uid is immutable, so the API
//     server rejects the patch when the live UID differs and no-ops when it matches.
//
// An empty uid means no guard was requested and patch is returned unchanged.
func applyUIDGuard(patchType types.PatchType, patch json.RawMessage, uid string) (json.RawMessage, error) {
	if uid == "" {
		return patch, nil
	}

	if patchType == types.JSONPatchType {
		var ops []json.RawMessage
		if err := json.Unmarshal(patch, &ops); err != nil {
			return nil, fmt.Errorf("invalid JSON patch: %w", err)
		}
		testOp, err := json.Marshal(map[string]string{"op": "test", "path": "/metadata/uid", "value": uid})
		if err != nil {
			return nil, err
		}
		return json.Marshal(append([]json.RawMessage{testOp}, ops...))
	}

	// strategic / merge patch: inject metadata.uid.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(patch, &obj); err != nil {
		return nil, fmt.Errorf("invalid patch body: %w", err)
	}
	// A JSON null (or scalar) unmarshals without error but leaves obj nil;
	// assigning into it below would panic ("assignment to entry in nil map").
	if obj == nil {
		return nil, errors.New("patch body must be a JSON object")
	}
	meta := map[string]json.RawMessage{}
	if raw, ok := obj["metadata"]; ok {
		if err := json.Unmarshal(raw, &meta); err != nil {
			return nil, fmt.Errorf("invalid patch metadata: %w", err)
		}
		// "metadata": null unmarshals to a nil map; guard before assigning uid.
		if meta == nil {
			return nil, errors.New("patch metadata must be a JSON object")
		}
	}
	uidJSON, err := json.Marshal(uid)
	if err != nil {
		return nil, err
	}
	meta["uid"] = uidJSON
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	obj["metadata"] = metaJSON
	return json.Marshal(obj)
}

// Execute applies a patch to a deployment using the specified strategy.
func (e *PatchDeploymentExecutor) Execute(ctx context.Context, in kubeactions.PatchDeploymentInputs) kubeactions.ExecutionResult {
	namespace := in.Namespace
	name := in.Name

	if len(in.Patch) == 0 {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: "patch is required for patch_deployment action",
		}
	}

	// Get the deployment first to verify UID matches resource_id.
	deployment, err := e.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to get deployment: %v", err),
		}
	}

	// resource_id is an optional UID guard: enforce only when supplied.
	if in.ResourceID != "" && string(deployment.UID) != in.ResourceID {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("deployment UID mismatch: expected %s, got %s - deployment may have been replaced since action was created", in.ResourceID, deployment.UID),
		}
	}

	patchType := resolvePatchType(in.PatchStrategy)
	patch, err := applyUIDGuard(patchType, in.Patch, in.ResourceID)
	if err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to build patch: %v", err),
		}
	}
	if _, err := e.clientset.AppsV1().Deployments(namespace).Patch(ctx, name, patchType, patch, metav1.PatchOptions{}); err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to patch deployment: %v", err),
		}
	}

	return kubeactions.ExecutionResult{
		Status:  kubeactions.StatusSuccess,
		Message: fmt.Sprintf("deployment %s/%s patched", namespace, name),
	}
}
