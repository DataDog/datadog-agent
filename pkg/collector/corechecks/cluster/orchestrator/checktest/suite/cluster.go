// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	vpafake "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	metafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// TestCluster is a fake Kubernetes API server: each object is served by the fake client of its API, which notifies the
// informers of the changes that the test makes.
type TestCluster struct {
	// APIClient holds the fake clients.
	APIClient *apiserver.APIClient
	// Resources are the resources that API discovery reports, beyond the built-in resources of the suite.
	Resources []metav1.APIResourceList
	// Versions returns the versions of a built-in resource that API discovery reports. Nil reports every version.
	Versions func(Resource) []string

	client   *fake.Clientset
	metadata *metafake.FakeMetadataClient
	crds     *apiextensionsfake.Clientset
	vpas     *vpafake.Clientset
	dynamic  *dynamicfake.FakeDynamicClient
}

// newTestCluster returns a fake API server that serves the objects. Unstructured objects must be passed at creation,
// so that the dynamic client can list their kind.
func newTestCluster(t testing.TB, objects []runtime.Object) *TestCluster {
	var typed, metadataOnly, crds, vpas, unstructuredObjects []runtime.Object
	listKinds := map[schema.GroupVersionResource]string{}
	for _, object := range objects {
		switch o := object.(type) {
		case *metav1.PartialObjectMetadata:
			metadataOnly = append(metadataOnly, o)
		case *apiextensionsv1.CustomResourceDefinition:
			crds = append(crds, o)
		case *vpav1.VerticalPodAutoscaler:
			vpas = append(vpas, o)
		case *unstructured.Unstructured:
			listKinds[resourceOf(o.GroupVersionKind())] = o.GetKind() + "List"
			unstructuredObjects = append(unstructuredObjects, o)
		default:
			typed = append(typed, o)
		}
	}

	metadataScheme := runtime.NewScheme()
	require.NoError(t, metav1.AddMetaToScheme(metadataScheme))
	c := &TestCluster{
		client:   fake.NewClientset(typed...),
		metadata: metafake.NewSimpleMetadataClient(metadataScheme, metadataOnly...),
		crds:     apiextensionsfake.NewSimpleClientset(crds...),
		vpas:     vpafake.NewSimpleClientset(vpas...), //nolint:staticcheck // SA1019 NewClientset not yet available in VPA
		dynamic:  dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds),
	}
	// The dynamic client guesses the resource of the objects passed at creation, which is wrong for some kinds.
	for _, object := range unstructuredObjects {
		c.Add(t, object)
	}
	c.client.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.0"}
	c.APIClient = &apiserver.APIClient{
		Cl:                 c.client,
		InformerCl:         c.client,
		DynamicCl:          c.dynamic,
		DynamicInformerCl:  c.dynamic,
		MetadataInformerCl: c.metadata,
		CRDInformerClient:  c.crds,
		VPAInformerClient:  c.vpas,
	}
	return c
}

func (c *TestCluster) Add(t testing.TB, object runtime.Object) {
	tracker, gvr := c.tracker(t, object)
	require.NoError(t, tracker.Create(gvr, object, namespace(t, object)))
}

func (c *TestCluster) Update(t testing.TB, object runtime.Object) {
	tracker, gvr := c.tracker(t, object)
	require.NoError(t, tracker.Update(gvr, object, namespace(t, object)))
}

// Delete deletes an object of the cluster, and returns its last state. Like the API server, it reports the deleted
// object with a new resource version, which the fake clients do not set.
func (c *TestCluster) Delete(t testing.TB, object runtime.Object) runtime.Object {
	tracker, gvr := c.tracker(t, object)
	accessor, err := meta.Accessor(object)
	require.NoError(t, err)
	stored, err := tracker.Get(gvr, accessor.GetNamespace(), accessor.GetName())
	require.NoError(t, err)
	deleted := stored.DeepCopyObject()
	deletedAccessor, err := meta.Accessor(deleted)
	require.NoError(t, err)
	deletedAccessor.SetResourceVersion(deletedAccessor.GetResourceVersion() + "-deleted")
	require.NoError(t, tracker.Update(gvr, deleted, accessor.GetNamespace()))
	// The tracker of the fake clients adds the managed fields of the update.
	last, err := tracker.Get(gvr, accessor.GetNamespace(), accessor.GetName())
	require.NoError(t, err)
	require.NoError(t, tracker.Delete(gvr, accessor.GetNamespace(), accessor.GetName()))
	return last
}

// ServerGroupsAndResources reports the built-in resources of the suite, in every version, and Resources.
func (c *TestCluster) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	// Group versions keep the order of Resources, so that the preferred version of a group is its first version.
	byGroupVersion := map[string]*metav1.APIResourceList{}
	var groupVersions []string
	add := func(groupVersion string, resource metav1.APIResource) {
		list, found := byGroupVersion[groupVersion]
		if !found {
			list = &metav1.APIResourceList{GroupVersion: groupVersion}
			byGroupVersion[groupVersion] = list
			groupVersions = append(groupVersions, groupVersion)
		}
		list.APIResources = append(list.APIResources, resource)
	}
	for _, resource := range Resources() {
		// The collectors of terminated pods and Helm releases read pods and ConfigMaps, which the API serves.
		if resource.IsCustomResource() || resource.Resource == "terminated-pods" || resource.Resource == "helm-releases-configmaps" {
			continue
		}
		versions := resource.Versions
		if c.Versions != nil {
			versions = c.Versions(resource)
		}
		for _, v := range versions {
			groupVersion := schema.GroupVersion{Group: resource.Group, Version: v}.String()
			add(groupVersion, metav1.APIResource{Name: resource.Resource, Kind: resource.Kind, Namespaced: true, Verbs: []string{"list", "watch"}})
		}
	}
	for _, list := range c.Resources {
		for _, resource := range list.APIResources {
			add(list.GroupVersion, resource)
		}
	}

	groupsByName := map[string]*metav1.APIGroup{}
	var groups []*metav1.APIGroup
	var lists []*metav1.APIResourceList
	for _, groupVersion := range groupVersions {
		lists = append(lists, byGroupVersion[groupVersion])
		gv, err := schema.ParseGroupVersion(groupVersion)
		if err != nil {
			return nil, nil, err
		}
		group, found := groupsByName[gv.Group]
		if !found {
			group = &metav1.APIGroup{Name: gv.Group}
			groupsByName[gv.Group] = group
			groups = append(groups, group)
		}
		versionForDiscovery := metav1.GroupVersionForDiscovery{GroupVersion: groupVersion, Version: gv.Version}
		group.Versions = append(group.Versions, versionForDiscovery)
		if group.PreferredVersion.Version == "" {
			group.PreferredVersion = versionForDiscovery
		}
	}
	return groups, lists, nil
}

// tracker returns the object tracker of the fake client that serves the object, and the object resource.
func (c *TestCluster) tracker(t testing.TB, object runtime.Object) (k8stesting.ObjectTracker, schema.GroupVersionResource) {
	var tracker k8stesting.ObjectTracker
	var gvk schema.GroupVersionKind
	switch o := object.(type) {
	case *metav1.PartialObjectMetadata:
		tracker, gvk = c.metadata.Tracker(), o.GroupVersionKind()
	case *unstructured.Unstructured:
		return c.dynamic.Tracker(), resourceOf(o.GroupVersionKind())
	case *apiextensionsv1.CustomResourceDefinition:
		tracker, gvk = c.crds.Tracker(), apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition")
	case *vpav1.VerticalPodAutoscaler:
		tracker, gvk = c.vpas.Tracker(), vpav1.SchemeGroupVersion.WithKind("VerticalPodAutoscaler")
	default:
		kinds, _, err := kscheme.Scheme.ObjectKinds(object)
		require.NoError(t, err)
		tracker, gvk = c.client.Tracker(), kinds[0]
	}
	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	return tracker, gvr
}

func namespace(t testing.TB, object runtime.Object) string {
	accessor, err := meta.Accessor(object)
	require.NoError(t, err)
	return accessor.GetNamespace()
}

// ServedVersion returns the version of the resource that the setup serves.
func ServedVersion(resource Resource, setup Setup) string {
	if !setup.OlderVersions {
		return resource.Versions[0]
	}
	return olderVersions()[resource.Group+"/"+resource.fixtureResource()]
}

// olderVersions returns the last version of each resource, like a cluster with older operators. A custom resource keeps
// its first version when another resource of its group serves it, as the check selects one version for each group.
var olderVersions = sync.OnceValue(func() map[string]string {
	versions := map[string]string{}
	for _, resource := range Resources() {
		versions[resource.Group+"/"+resource.fixtureResource()] = resource.Versions[len(resource.Versions)-1]
	}
	for changed := true; changed; {
		changed = false
		for _, resource := range Resources() {
			key := resource.Group + "/" + resource.fixtureResource()
			if !resource.IsCustomResource() || versions[key] == resource.Versions[0] {
				continue
			}
			for _, other := range Resources() {
				otherKey := other.Group + "/" + other.fixtureResource()
				if other.Group == resource.Group && otherKey != key && versions[otherKey] == resource.Versions[0] {
					versions[key], changed = resource.Versions[0], true
					break
				}
			}
		}
	}
	return versions
})

// customResourceDiscovery returns what API discovery reports for the custom resources. Discovery reports the built-in
// resources itself.
func customResourceDiscovery(objects []*Object, setup Setup) []metav1.APIResourceList {
	var lists []metav1.APIResourceList
	for _, o := range objects {
		if o.APIResource == nil || slices.Contains(setup.Undiscovered, o.FullName) {
			continue
		}
		groupVersion := o.FullName[:strings.LastIndex(o.FullName, "/")]
		index := slices.IndexFunc(lists, func(l metav1.APIResourceList) bool { return l.GroupVersion == groupVersion })
		if index < 0 {
			lists = append(lists, metav1.APIResourceList{GroupVersion: groupVersion})
			index = len(lists) - 1
		}
		lists[index].APIResources = append(lists[index].APIResources, *o.APIResource)
	}
	return lists
}

// fakeDiscovery serves the API discovery of the fake cluster, or the error that a test sets.
type fakeDiscovery struct {
	cluster *TestCluster
	mu      sync.Mutex
	err     error
}

func (d *fakeDiscovery) fail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.err = err
}

func (d *fakeDiscovery) failing() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err != nil
}

func (d *fakeDiscovery) serverGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	d.mu.Lock()
	err := d.err
	d.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	return d.cluster.ServerGroupsAndResources()
}
