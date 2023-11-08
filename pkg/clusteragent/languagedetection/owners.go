// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

// Package languagedetection contains the DCA handler functions to patch kubernetes resources with language annotations
package languagedetection

import (
	"fmt"
	"strings"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	kindDeployment = "Deployment"
	kindReplicaset = "ReplicaSet"
)

// Currently only deployments are supported
var supportedBaseOwners = map[string]struct{}{
	kindDeployment: {},
}

// NamespacedOwnerReference defines an owner reference bound to a namespace
type NamespacedOwnerReference struct {
	metav1.OwnerReference
	namespace string
}

// NewNamespacedOwnerReference returns a new namespaced owner reference
func NewNamespacedOwnerReference(apiVersion string, kind string, name string, uid string, namespace string) NamespacedOwnerReference {
	return NamespacedOwnerReference{
		OwnerReference: metav1.OwnerReference{
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       name,
			UID:        types.UID(uid),
		},
		namespace: namespace,
	}
}

// getNamespacedBaseOwnerReference creates a new namespaced owner reference object representing the base owner of the pod
// In case the first owner's kind is replicaset, it returns an owner reference to the parent deployment
// of the replicaset
func getNamespacedBaseOwnerReference(podDetails *pbgo.PodLanguageDetails) NamespacedOwnerReference {
	ownerref := podDetails.Ownerref
	kind := ownerref.Kind
	name := ownerref.Name
	uid := ownerref.Id

	// This should be included in the KubeOwnerInfo by the client.
	// For now, it is hard-coded, and we support apps/v1 strictly
	apiVersion := "apps/v1"

	if kind == kindReplicaset {
		kind = kindDeployment
		name = kubernetes.ParseDeploymentForReplicaSet(name)
	}

	return NamespacedOwnerReference{
		OwnerReference: metav1.OwnerReference{
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       name,
			UID:        types.UID(uid),
		},
		namespace: podDetails.Namespace,
	}
}

// getGVR returns the GroupVersionResource of the ownerRef
func getGVR(namespacedOwnerRef *NamespacedOwnerReference) (schema.GroupVersionResource, error) {
	gv, err := schema.ParseGroupVersion(namespacedOwnerRef.APIVersion)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	gvr := gv.WithResource(fmt.Sprintf("%ss", strings.ToLower(namespacedOwnerRef.Kind)))

	return gvr, nil
}
