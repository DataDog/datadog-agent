// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"strconv"
)

const (
	unset = int64(-1)

	// The following were sourced from https://github.com/kubernetes/kubernetes/blob/45836971f27ca70cd7742e8ee66e99e3c648cf9f/staging/src/k8s.io/kubectl/pkg/util/deployment/deployment.go
	revisionAnnotation        = "deployment.kubernetes.io/revision"
	revisionHistoryAnnotation = "deployment.kubernetes.io/revision-history"
	desiredReplicasAnnotation = "deployment.kubernetes.io/desired-replicas"
	maxReplicasAnnotation     = "deployment.kubernetes.io/max-replicas"
)

// Code sourced from https://github.com/kubernetes/kubernetes/blob/186a326fb3a9cd0dd91098cc94b9f1f8f1536ed3/staging/src/k8s.io/kubectl/pkg/polymorphichelpers/rollback.go
var annotationsToSkip = map[string]bool{
	corev1.LastAppliedConfigAnnotation: true,
	revisionAnnotation:                 true,
	revisionHistoryAnnotation:          true,
	desiredReplicasAnnotation:          true,
	maxReplicasAnnotation:              true,
	appsv1.DeprecatedRollbackTo:        true,
}

// Code sourced from https://github.com/kubernetes/kubernetes/blob/186a326fb3a9cd0dd91098cc94b9f1f8f1536ed3/staging/src/k8s.io/kubectl/pkg/polymorphichelpers/rollback.go
func getDeploymentPatch(podTemplate *corev1.PodTemplateSpec, annotations map[string]string) (types.PatchType, []byte, error) {
	// Create a patch of the Deployment that replaces spec.template
	patch, err := json.Marshal([]interface{}{
		map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/template",
			"value": podTemplate,
		},
		map[string]interface{}{
			"op":    "replace",
			"path":  "/metadata/annotations",
			"value": annotations,
		},
	})
	return types.JSONPatchType, patch, err
}

func getPatchAnnotations(deployment *appsv1.Deployment, replicaSet *appsv1.ReplicaSet) map[string]string {
	merged := make(map[string]string)
	// grab all the annotationsToSkip from the current deployment
	for key := range annotationsToSkip {
		if val, ok := deployment.Annotations[key]; ok {
			merged[key] = val
		}
	}

	// grab everything EXCEPT annotationsToSKip from the replicaset revision
	for key, val := range replicaSet.Annotations {
		if !annotationsToSkip[key] {
			merged[key] = val
		}
	}
	return merged
}

type RollbackDeploymentExecutor struct {
	clientset kubernetes.Interface
}

// NewRollbackDeploymentExecutor creates a new RollbackDeploymentExecutor
func NewRollbackDeploymentExecutor(clientset kubernetes.Interface) *RollbackDeploymentExecutor {
	return &RollbackDeploymentExecutor{
		clientset: clientset,
	}
}

// validateTargetDeployment checks that
// 1. deployment exists
// 2. deployment's UID matches what was passed
// 3. deployment is not paused
func validateTargetDeployment(ctx context.Context, client kubernetes.Interface, name string, namespace string, uid string) (*appsv1.Deployment, error) {
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, log.Errorf("failed to get deployment %s/%s: %v", namespace, name, err)
	}

	if string(deployment.UID) != uid {
		return nil, log.Errorf("deployment %s/%s UID mismatch: expected %s, got %s - deployment may have been replaced", namespace, name, uid, deployment.UID)
	}

	if deployment.Spec.Paused {
		return nil, log.Errorf("deployment %s/%s is paused, cannot perform a rollback", namespace, name)
	}

	return deployment, nil
}

// getReplicaSetByRevision returns a ReplicaSet for a given revision, if nothing is passed (i.e. targetRevision is 0)
// the default behavior matches kubectl and returns the previous revision
func getReplicaSetByRevision(ctx context.Context, client kubernetes.Interface, deployment *appsv1.Deployment, targetRevision int64) (int64, *appsv1.ReplicaSet, error) {
	namespace := deployment.Namespace
	name := deployment.Name

	// Revisions can never be negative, they start at 1
	if targetRevision < 0 {
		return unset, nil, log.Errorf("revision %d is invalid", targetRevision)
	}

	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return unset, nil, log.Errorf("failed to build deployment label selector for %s/%s", namespace, name)
	}

	// List all ReplicaSets with a label selector matching the deployments
	rsInterface := client.AppsV1().ReplicaSets(namespace)
	allReplicaSets, err := rsInterface.List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return unset, nil, log.Errorf("failed to list replicasets in namespace %s", namespace)
	}

	// Revisions are not always sequential or ordered
	currentRevision := unset
	previousRevision := unset
	currentRevisionIndex := int(unset)
	previousRevisionIndex := int(unset)

	// Loop over all ReplicaSets
	for i, rs := range allReplicaSets.Items {
		// Ensure that the owner of the ReplicaSet is the current deployment
		if metav1.IsControlledBy(&rs, deployment) {
			// Check and parse the revision from the "deployment.kubernetes.io/revision" annotation
			if revision, ok := rs.Annotations[revisionAnnotation]; ok {
				rev, err := strconv.ParseInt(revision, 10, 64)
				if err != nil {
					log.Errorf("failed to parse replicaset revision from %s", revision)
					continue
				}

				// if targetRevision is 0 then the user is requesting a rollback to the previous revision
				if targetRevision == 0 {
					// previousRevision < currentRevision < rev
					if currentRevision < rev {
						previousRevision = currentRevision
						previousRevisionIndex = currentRevisionIndex

						currentRevision = rev
						currentRevisionIndex = i
					} else if previousRevision < rev {
						// previousRevision < rev < currentRevision
						previousRevision = rev
						previousRevisionIndex = i
					}
				} else if targetRevision == rev {
					// User supplied targetRevision was found, return early
					return targetRevision, &allReplicaSets.Items[i], nil
				}
			}
		}
	}

	// If targetRevision is > 0, and we didn't return early, that means it wasn't found
	if targetRevision > 0 {
		return unset, nil, log.Errorf("failed to find revision %d", targetRevision)
	}

	// If previous index is -1 that means there is one (or none) revisions
	if previousRevisionIndex == int(unset) {
		return unset, nil, log.Errorf("failed to find revision %d", targetRevision)
	}

	// Return the previous revision by default
	return previousRevision, &allReplicaSets.Items[previousRevisionIndex], nil
}

// Execute rolls back a deployment to a previous revision by patching the current deployment
func (r *RollbackDeploymentExecutor) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	resource := action.Resource
	namespace := resource.Namespace
	name := resource.Name
	resourceID := resource.ResourceId
	targetRevision := action.GetRollbackDeployment().GetTargetRevision()

	currentDeployment, err := validateTargetDeployment(ctx, r.clientset, name, namespace, resourceID)
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: err.Error(),
		}
	}

	foundRevision, replicaSetForRevision, err := getReplicaSetByRevision(ctx, r.clientset, currentDeployment, targetRevision)
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: err.Error(),
		}
	}

	// Delete pod-template-hash label from both the current Deployment Spec and ReplicaSet Spec Templates
	// These labels interfere with equality evaluation and patching
	delete(currentDeployment.Spec.Template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
	delete(replicaSetForRevision.Spec.Template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
	if apiequality.Semantic.DeepEqual(&replicaSetForRevision.Spec.Template, &currentDeployment.Spec.Template) {
		// User supplied a specific revision
		msg := fmt.Sprintf("current template already matches revision %d", targetRevision)
		if targetRevision == 0 {
			// User fell back on default behavior
			msg = fmt.Sprintf("current template already matches the previous revision %d", foundRevision)
		}
		return ExecutionResult{
			Status:  StatusSuccess,
			Message: msg,
		}
	}

	// Create the patch by merging the current Deployment's annotations with the target revision's
	patchAnnotations := getPatchAnnotations(currentDeployment, replicaSetForRevision)
	patchType, patch, err := getDeploymentPatch(&replicaSetForRevision.Spec.Template, patchAnnotations)
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: err.Error(),
		}
	}

	// Restore revision by patching the Deployment
	if _, err = r.clientset.AppsV1().Deployments(namespace).Patch(ctx, name, patchType, patch, metav1.PatchOptions{}); err != nil {
		msg := fmt.Sprintf("failed restoring revision %d: %v", foundRevision, err)
		log.Error(msg)
		return ExecutionResult{
			Status:  StatusFailed,
			Message: msg,
		}

	}

	return ExecutionResult{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("successfully restored revision %d", foundRevision),
	}
}
