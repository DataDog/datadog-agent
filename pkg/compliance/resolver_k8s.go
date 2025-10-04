// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package compliance

import (
	"context"
	"fmt"

	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeunstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kubeschema "k8s.io/apimachinery/pkg/runtime/schema"
	kubedynamic "k8s.io/client-go/dynamic"
)

// KubernetesGroupsAndResourcesProvider is a function that returns the Kubernetes groups and services
// Note: this is the same as the ServerGroupsAndResources function defined in
// k8s.io/client-go/discovery. It is redefined here to avoid a direct dependency
// on k8s.io/client-go/discovery which substantially increases the size of the
// security agent binary.
type KubernetesGroupsAndResourcesProvider func() ([]*kubemetav1.APIGroup, []*kubemetav1.APIResourceList, error)

// KubernetesProvider is a function returning a Kubernetes client.
type KubernetesProvider func(context.Context) (kubedynamic.Interface, KubernetesGroupsAndResourcesProvider, error)

type k8sapiserverResolver struct {
	kubeResourcesCache *[]*kubemetav1.APIResourceList
	kubeClusterIDCache string

	kubernetesCl                    kubedynamic.Interface
	kubernetesGroupAndResourcesFunc KubernetesGroupsAndResourcesProvider
}

func newK8sapiserverResolver(ctx context.Context, opts ResolverOptions) *k8sapiserverResolver {
	r := &k8sapiserverResolver{}

	if opts.KubernetesProvider != nil {
		r.kubernetesCl, r.kubernetesGroupAndResourcesFunc, _ = opts.KubernetesProvider(ctx)
	}
	return r
}

func (r *k8sapiserverResolver) close() {
	r.kubernetesCl = nil
	r.kubernetesGroupAndResourcesFunc = nil

	r.kubeClusterIDCache = ""
	r.kubeResourcesCache = nil
}

func (r *k8sapiserverResolver) isEnabled() bool {
	return r.kubernetesCl != nil
}

func (r *k8sapiserverResolver) resolveKubeClusterID(ctx context.Context) string {
	if r.kubeClusterIDCache == "" {
		cl := r.kubernetesCl
		if cl == nil {
			return ""
		}

		resourceDef := cl.Resource(kubeschema.GroupVersionResource{
			Resource: "namespaces",
			Version:  "v1",
		})
		resource, err := resourceDef.Get(ctx, "kube-system", kubemetav1.GetOptions{})
		if err != nil {
			return ""
		}
		r.kubeClusterIDCache = string(resource.GetUID())
	}
	return r.kubeClusterIDCache
}

func (r *k8sapiserverResolver) resolveKubeApiserver(ctx context.Context, spec InputSpecKubeapiserver) (interface{}, error) {
	cl := r.kubernetesCl
	if cl == nil {
		return nil, ErrIncompatibleEnvironment
	}

	if len(spec.Kind) == 0 {
		return nil, fmt.Errorf("cannot run Kubeapiserver check, resource kind is empty")
	}

	if len(spec.APIRequest.Verb) == 0 {
		return nil, fmt.Errorf("cannot run Kubeapiserver check, action verb is empty")
	}

	if len(spec.Version) == 0 {
		spec.Version = "v1"
	}

	// podsecuritypolicies have been deprecated as part of Kubernetes v1.25

	resourceSchema := kubeschema.GroupVersionResource{
		Group:    spec.Group,
		Resource: spec.Kind,
		Version:  spec.Version,
	}

	resourceSupported, err := r.checkKubeServerResourceSupport(resourceSchema)
	if err != nil {
		return nil, fmt.Errorf("unable to check for Kube resource support:'%v', ns:'%s' err: %w",
			resourceSchema, spec.Namespace, err)
	}
	if !resourceSupported {
		return nil, ErrIncompatibleEnvironment
	}

	resourceDef := cl.Resource(resourceSchema)
	var resourceAPI kubedynamic.ResourceInterface
	if len(spec.Namespace) > 0 {
		resourceAPI = resourceDef.Namespace(spec.Namespace)
	} else {
		resourceAPI = resourceDef
	}

	var items []kubeunstructured.Unstructured
	api := spec.APIRequest
	switch api.Verb {
	case "get":
		if len(api.ResourceName) == 0 {
			return nil, fmt.Errorf("unable to use 'get' apirequest without resource name")
		}
		resource, err := resourceAPI.Get(ctx, spec.APIRequest.ResourceName, kubemetav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("unable to get Kube resource:'%v', ns:'%s' name:'%s', err: %v",
				resourceSchema, spec.Namespace, api.ResourceName, err)
		}
		items = []kubeunstructured.Unstructured{*resource}
	case "list":
		list, err := resourceAPI.List(ctx, kubemetav1.ListOptions{
			LabelSelector: spec.LabelSelector,
			FieldSelector: spec.FieldSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to list Kube resources:'%v', ns:'%s' name:'%s', err: %v",
				resourceSchema, spec.Namespace, api.ResourceName, err)
		}
		items = list.Items
	}

	resolved := make([]interface{}, 0, len(items))
	for _, resource := range items {
		resolved = append(resolved, map[string]interface{}{
			"kind":      resource.GetObjectKind().GroupVersionKind().Kind,
			"group":     resource.GetObjectKind().GroupVersionKind().Group,
			"version":   resource.GetObjectKind().GroupVersionKind().Version,
			"namespace": resource.GetNamespace(),
			"name":      resource.GetName(),
			"resource":  resource,
		})
	}
	return resolved, nil
}

func (r *k8sapiserverResolver) checkKubeServerResourceSupport(resourceSchema kubeschema.GroupVersionResource) (bool, error) {
	if r.kubernetesGroupAndResourcesFunc == nil {
		return true, nil
	}

	if r.kubeResourcesCache == nil {
		_, resources, err := r.kubernetesGroupAndResourcesFunc()
		if err != nil {
			return false, fmt.Errorf("could not fetch kubernetes resources: %w", err)
		}
		r.kubeResourcesCache = &resources
	}

	groupVersion := resourceSchema.GroupVersion().String()
	for _, list := range *r.kubeResourcesCache {
		if groupVersion == list.GroupVersion {
			for _, r := range list.APIResources {
				if r.Name == resourceSchema.Resource {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
