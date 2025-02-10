// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	// KindDeployment represents the deployment resource kind
	KindDeployment = "Deployment"
	// KindReplicaset represents the replicaset resource kind
	KindReplicaset = "ReplicaSet"
)

// SupportedBaseOwners is the set of owner resource types supported by language detection
// Currently only deployments are supported
// For KindReplicaset, we use the parent deployment
var SupportedBaseOwners = map[string]struct{}{
	KindDeployment: {},
}

// NamespacedOwnerReference defines an owner reference bound to a namespace
type NamespacedOwnerReference struct {
	Name       string
	APIVersion string
	Kind       string
	Namespace  string
}

// NewNamespacedOwnerReference returns a new namespaced owner reference
func NewNamespacedOwnerReference(apiVersion string, kind string, name string, namespace string) NamespacedOwnerReference {
	return NamespacedOwnerReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  namespace,
	}
}

// GetNamespacedBaseOwnerReference creates a new namespaced owner reference object representing the base owner of the pod
// In case the first owner's kind is replicaset, it returns an owner reference to the parent deployment
// of the replicaset
func GetNamespacedBaseOwnerReference(kind, name, namespace string) NamespacedOwnerReference {
	// This should be included in the KubeOwnerInfo by the client.
	// For now, it is hard-coded, and we support apps/v1 strictly
	apiVersion := "apps/v1"
	if kind == KindReplicaset {
		kind = KindDeployment
		name = kubernetes.ParseDeploymentForReplicaSet(name)
	}

	return NamespacedOwnerReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  namespace,
	}
}

// GetGVR returns the GroupVersionResource of the ownerRef
func GetGVR(namespacedOwnerRef *NamespacedOwnerReference) (schema.GroupVersionResource, error) {
	gv, err := schema.ParseGroupVersion(namespacedOwnerRef.APIVersion)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	gvr := gv.WithResource(fmt.Sprintf("%ss", strings.ToLower(namespacedOwnerRef.Kind)))

	return gvr, nil
}
