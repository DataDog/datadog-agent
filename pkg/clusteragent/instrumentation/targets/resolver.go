// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0. This product includes software developed
// at Datadog (https://www.datadoghq.com/). Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package targets

import (
	"context"
	"fmt"
	"sync"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const maxOwnerChainDepth = 16

type cachedOwner struct {
	owners []metav1.OwnerReference
}

// Resolver follows configured Kubernetes owner-reference paths and returns the
// registered workload targets found in a Pod's ownership chain.
type Resolver struct {
	registry *Registry
	client   dynamic.Interface

	cacheMutex sync.RWMutex
	cache      map[string]cachedOwner
}

// NewResolver creates an owner-chain resolver.
func NewResolver(registry *Registry, client dynamic.Interface) *Resolver {
	return &Resolver{
		registry: registry,
		client:   client,
		cache:    make(map[string]cachedOwner),
	}
}

// Resolve returns all registered workload targets encountered from the Pod's
// controller owner upward. Resolution stops at an unregistered owner kind or
// an API read failure.
func (r *Resolver) Resolve(ctx context.Context, pod *workloadmeta.KubernetesPod) ([]workloadmeta.KubernetesMatchedOwner, error) {
	if r == nil || r.registry == nil || r.client == nil || pod == nil {
		return nil, nil
	}

	owner, found := controllerOwner(pod.Owners)
	if !found {
		return nil, nil
	}

	resolved := make([]workloadmeta.KubernetesMatchedOwner, 0, 2)
	for depth := 0; depth < maxOwnerChainDepth; depth++ {
		descriptor, registered := r.registry.descriptor(owner.APIVersion, owner.Kind)
		if !registered {
			break
		}

		if descriptor.isTarget {
			gv, _ := schema.ParseGroupVersion(owner.APIVersion)
			resolved = append(resolved, workloadmeta.KubernetesMatchedOwner{
				Group:     gv.Group,
				Version:   gv.Version,
				Kind:      owner.Kind,
				Namespace: pod.Namespace,
				Name:      owner.Name,
			})
		}

		if !descriptor.traversable {
			break
		}
		nextOwners, err := r.getOwners(ctx, descriptor.resource, pod.Namespace, owner.Name, owner.ID)
		if err != nil {
			return resolved, err
		}
		owner, found = metav1ControllerOwner(nextOwners)
		if !found {
			break
		}
	}
	return resolved, nil
}

func (r *Resolver) getOwners(ctx context.Context, resource Resource, namespace, name, uid string) ([]metav1.OwnerReference, error) {
	cacheKey := resourceKey(resource.APIVersion, resource.Kind) + "/" + namespace + "/" + name + "/" + uid
	r.cacheMutex.RLock()
	cached, found := r.cache[cacheKey]
	r.cacheMutex.RUnlock()
	if found {
		return cached.owners, nil
	}

	gv, err := schema.ParseGroupVersion(resource.APIVersion)
	if err != nil {
		return nil, err
	}
	obj, err := r.client.Resource(schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: resource.Resource,
	}).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("resolve owner %s %s/%s: %w", resourceKey(resource.APIVersion, resource.Kind), namespace, name, err)
	}
	if uid != "" && string(obj.GetUID()) != uid {
		return nil, fmt.Errorf("resolve owner %s %s/%s: UID changed from %s to %s", resourceKey(resource.APIVersion, resource.Kind), namespace, name, uid, obj.GetUID())
	}

	owners := obj.GetOwnerReferences()
	r.cacheMutex.Lock()
	r.cache[cacheKey] = cachedOwner{owners: owners}
	r.cacheMutex.Unlock()
	return owners, nil
}

func controllerOwner(owners []workloadmeta.KubernetesPodOwner) (workloadmeta.KubernetesPodOwner, bool) {
	if len(owners) == 0 {
		return workloadmeta.KubernetesPodOwner{}, false
	}
	for _, owner := range owners {
		if owner.Controller != nil && *owner.Controller {
			return owner, true
		}
	}
	return owners[0], true
}

func metav1ControllerOwner(owners []metav1.OwnerReference) (workloadmeta.KubernetesPodOwner, bool) {
	if len(owners) == 0 {
		return workloadmeta.KubernetesPodOwner{}, false
	}
	selected := owners[0]
	for _, owner := range owners {
		if owner.Controller != nil && *owner.Controller {
			selected = owner
			break
		}
	}
	return workloadmeta.KubernetesPodOwner{
		APIVersion: selected.APIVersion,
		Kind:       selected.Kind,
		Name:       selected.Name,
		ID:         string(selected.UID),
		Controller: selected.Controller,
	}, true
}
