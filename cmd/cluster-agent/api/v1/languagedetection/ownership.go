// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// authoritativeDeploymentOwnerForPodLanguages resolves the Deployment that should receive
// language detection data for a given pod, using only Kubernetes state already present in
// workloadmeta (from kube-apiserver watches).
//
// It mitigates forged owner references in client payloads by ignoring the request's owner
// and re-deriving the parent Deployment from the live Pod object in the store:
//   - the pod's unique controller owner must be a ReplicaSet (pods that reference a
//     Deployment directly are rejected),
//   - the pod name must be consistent with that owner: a Deployment pod is named
//     "<deployment>-<replicaset-hash>-<pod-hash>", so the Deployment parsed from the pod
//     name must match the Deployment parsed from the ReplicaSet owner. Kubernetes does not
//     enforce this consistency, so a forged pod (arbitrary name pointing at another
//     workload's ReplicaSet) is rejected here,
//   - the resolved Deployment must exist in the store.
//
// The second return value is false when the pod cannot be attributed safely.
func authoritativeDeploymentOwnerForPodLanguages(wlm workloadmeta.Component, podNamespace, podName string) (langUtil.NamespacedOwnerReference, bool) {
	pod, err := wlm.GetKubernetesPodByName(podName, podNamespace)
	if err != nil || pod == nil {
		log.Debugf("language detection: skipping pod %s/%s: workloadmeta pod lookup failed: %v", podNamespace, podName, err)
		return langUtil.NamespacedOwnerReference{}, false
	}

	ctrl := findUniqueControllerOwner(pod.Owners)
	if ctrl == nil {
		log.Debugf("language detection: skipping pod %s/%s: no unique controller owner in workloadmeta", podNamespace, podName)
		return langUtil.NamespacedOwnerReference{}, false
	}

	// Deployment-managed pods are owned by a ReplicaSet controller, not by the Deployment directly.
	if ctrl.Kind != langUtil.KindReplicaset {
		log.Debugf("language detection: skipping pod %s/%s: controller kind is %q, expected ReplicaSet", podNamespace, podName, ctrl.Kind)
		return langUtil.NamespacedOwnerReference{}, false
	}
	rsGroup := ctrl.Group
	if rsGroup == "" {
		// Older objects may omit apiVersion; ReplicaSet is always under apps (or legacy extensions).
		rsGroup = "apps"
	}
	if rsGroup != "apps" && rsGroup != "extensions" {
		log.Debugf("language detection: skipping pod %s/%s: ReplicaSet group %q is not supported", podNamespace, podName, rsGroup)
		return langUtil.NamespacedOwnerReference{}, false
	}

	deploymentName := kubernetes.ParseDeploymentForReplicaSet(ctrl.Name)
	if deploymentName == "" {
		log.Debugf("language detection: skipping pod %s/%s: could not parse deployment from ReplicaSet %q", podNamespace, podName, ctrl.Name)
		return langUtil.NamespacedOwnerReference{}, false
	}

	// Cross-check the pod name against the owner reference. The Deployment parsed from the
	// pod name must match the Deployment parsed from the (validated) ReplicaSet owner.
	if fromName := kubernetes.ParseDeploymentForPodName(podName); fromName != deploymentName {
		log.Debugf("language detection: skipping pod %s/%s: pod name resolves to deployment %q but owner reference resolves to %q", podNamespace, podName, fromName, deploymentName)
		return langUtil.NamespacedOwnerReference{}, false
	}

	deploymentID := podNamespace + "/" + deploymentName
	if _, err := wlm.GetKubernetesDeployment(deploymentID); err != nil {
		log.Debugf("language detection: skipping pod %s/%s: deployment %q not found in workloadmeta: %v", podNamespace, podName, deploymentID, err)
		return langUtil.NamespacedOwnerReference{}, false
	}

	return langUtil.NewNamespacedOwnerReference("apps/v1", langUtil.KindDeployment, deploymentName, podNamespace), true
}

// findUniqueControllerOwner returns the single owner reference marked as controller.
// It returns nil if there is no controller owner or more than one.
func findUniqueControllerOwner(owners []workloadmeta.KubernetesPodOwner) *workloadmeta.KubernetesPodOwner {
	var found *workloadmeta.KubernetesPodOwner
	for i := range owners {
		owner := &owners[i]
		if owner.Controller == nil || !*owner.Controller {
			continue
		}
		if found != nil {
			return nil
		}
		found = owner
	}
	return found
}
