// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// deploymentOwnerForPod resolves the Deployment that a pod's detected languages should be
// attributed to, from the reported owner reference.
//
// A pod created by a Deployment is owned by a ReplicaSet and named
// "<deployment>-<replicaset-hash>-<pod-hash>". We therefore require:
//   - the owner to be a ReplicaSet (pods that reference a Deployment directly are rejected), and
//   - the Deployment parsed from the pod name to match the Deployment parsed from the
//     ReplicaSet owner name.
func deploymentOwnerForPod(podDetail *pbgo.PodLanguageDetails) (langUtil.NamespacedOwnerReference, bool) {
	owner := podDetail.Ownerref
	if owner == nil || owner.Kind != langUtil.KindReplicaset {
		log.Debugf("language detection: skipping pod %s/%s: owner is not a ReplicaSet", podDetail.Namespace, podDetail.Name)
		return langUtil.NamespacedOwnerReference{}, false
	}

	deploymentName := kubernetes.ParseDeploymentForReplicaSet(owner.Name)
	if deploymentName == "" {
		log.Debugf("language detection: skipping pod %s/%s: could not parse deployment from ReplicaSet %q", podDetail.Namespace, podDetail.Name, owner.Name)
		return langUtil.NamespacedOwnerReference{}, false
	}

	if fromName := kubernetes.ParseDeploymentForPodName(podDetail.Name); fromName != deploymentName {
		log.Debugf("language detection: skipping pod %s/%s: pod name resolves to deployment %q but owner reference resolves to %q", podDetail.Namespace, podDetail.Name, fromName, deploymentName)
		return langUtil.NamespacedOwnerReference{}, false
	}

	return langUtil.NewNamespacedOwnerReference("apps/v1", langUtil.KindDeployment, deploymentName, podDetail.Namespace), true
}
