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

// deploymentOwnerForPod resolves the namespaced base owner that a pod's detected languages
// should be attributed to, reusing GetNamespacedBaseOwnerReference, but only after two
// consistency checks on the reported owner reference:
//   - the owner must be a ReplicaSet (pods that reference a Deployment, or any other resource,
//     directly are rejected), and
//   - the ReplicaSet parsed from the pod name must match the owner ReplicaSet name exactly, so a
//     pod cannot be attributed to a ReplicaSet it does not belong to even if both names share the
//     same Deployment prefix.
func deploymentOwnerForPod(podDetail *pbgo.PodLanguageDetails) (langUtil.NamespacedOwnerReference, bool) {
	owner := podDetail.Ownerref
	if owner == nil || owner.Kind != langUtil.KindReplicaset {
		log.Debugf("language detection: skipping pod %s/%s: owner is not a ReplicaSet", podDetail.Namespace, podDetail.Name)
		return langUtil.NamespacedOwnerReference{}, false
	}

	if rsFromName := kubernetes.ParseReplicaSetForPodName(podDetail.Name); rsFromName != owner.Name {
		log.Debugf("language detection: skipping pod %s/%s: pod name resolves to ReplicaSet %q but owner reference is %q", podDetail.Namespace, podDetail.Name, rsFromName, owner.Name)
		return langUtil.NamespacedOwnerReference{}, false
	}

	return langUtil.GetNamespacedBaseOwnerReference(podDetail), true
}
