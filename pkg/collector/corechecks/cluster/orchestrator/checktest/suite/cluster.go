// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	vpafake "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	metafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// fakeCluster is a fake Kubernetes API server. Each object is served by the fake client of its API, which notifies the
// informers of the changes that a test makes.
type fakeCluster struct {
	apiClient *apiserver.APIClient

	client   *fake.Clientset
	metadata *metafake.FakeMetadataClient
	crds     *apiextensionsfake.Clientset
	vpas     *vpafake.Clientset
	dynamic  *dynamicfake.FakeDynamicClient

	mu sync.Mutex
	// discovered are the resources that API discovery reports, in their order. Group versions keep this order, so the
	// preferred version of a group is its first one.
	discovered   []discoveredResource
	discoveryErr error
	// failedGroupVersions are the group versions whose resources discovery does not report, with a partial failure.
	failedGroupVersions []string
	// previous are the states of the objects before each update, by the resource version of the update, so that a
	// watch with selectors reports an object that starts or stops matching them, as the API server does.
	previous map[string]runtime.Object
	// objects are the objects of the cluster, in their current state, which every check of a test shares.
	objects []Fixture
}

type discoveredResource struct {
	groupVersion string
	resource     metav1.APIResource
}

func newFakeCluster(t testing.TB) *fakeCluster {
	metadataScheme := runtime.NewScheme()
	require.NoError(t, metav1.AddMetaToScheme(metadataScheme))
	listKinds := map[schema.GroupVersionResource]string{}
	for _, f := range append(customResourceFixtures(), userCustomResourceFixtures()...) {
		listKinds[f.gvr] = f.Discovery.Kind + "List"
	}
	listKinds[endpointSlices] = "EndpointSliceList"

	c := &fakeCluster{
		client:   fake.NewClientset(),
		metadata: metafake.NewSimpleMetadataClient(metadataScheme),
		crds:     apiextensionsfake.NewSimpleClientset(),
		vpas:     vpafake.NewSimpleClientset(), //nolint:staticcheck // SA1019 NewClientset not yet available in VPA
		dynamic:  dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds),
		previous: map[string]runtime.Object{},
	}
	c.client.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.0"}
	c.serveWatches(&c.client.Fake, c.client.Tracker())
	c.serveWatches(&c.metadata.Fake, c.metadata.Tracker())
	c.serveWatches(&c.crds.Fake, c.crds.Tracker())
	c.serveWatches(&c.vpas.Fake, c.vpas.Tracker())
	c.serveWatches(&c.dynamic.Fake, c.dynamic.Tracker())
	serveFieldSelectors(&c.client.Fake, c.client.Tracker())
	c.apiClient = &apiserver.APIClient{
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

// add adds an object to the cluster. Like the API server, the metadata client also serves ConfigMaps and Secrets.
func (c *fakeCluster) add(t testing.TB, object runtime.Object) {
	for _, o := range c.served(object) {
		tracker, gvr := c.tracker(t, o)
		require.NoError(t, tracker.Create(gvr, o, namespaceOf(t, o)))
	}
}

func (c *fakeCluster) update(t testing.TB, object runtime.Object) {
	for _, o := range c.served(object) {
		tracker, gvr := c.tracker(t, o)
		accessor, err := meta.Accessor(o)
		require.NoError(t, err)
		previous, err := tracker.Get(gvr, accessor.GetNamespace(), accessor.GetName())
		require.NoError(t, err)
		c.mu.Lock()
		c.previous[objectKey(tracker, gvr, accessor.GetNamespace(), accessor.GetName(), accessor.GetResourceVersion())] = previous
		c.mu.Unlock()
		require.NoError(t, tracker.Update(gvr, o, accessor.GetNamespace()))
	}
}

func (c *fakeCluster) delete(t testing.TB, object runtime.Object) {
	for _, o := range c.served(object) {
		tracker, gvr := c.tracker(t, o)
		accessor, err := meta.Accessor(o)
		require.NoError(t, err)
		require.NoError(t, tracker.Delete(gvr, accessor.GetNamespace(), accessor.GetName()))
	}
}

// served returns the forms in which the clients serve an object: a ConfigMap or a Secret is also served by the
// metadata client.
func (c *fakeCluster) served(object runtime.Object) []runtime.Object {
	var objectMeta metav1.ObjectMeta
	var kind string
	switch o := object.(type) {
	case *corev1.ConfigMap:
		objectMeta, kind = o.ObjectMeta, "ConfigMap"
	case *corev1.Secret:
		objectMeta, kind = o.ObjectMeta, "Secret"
	default:
		return []runtime.Object{object}
	}
	metadataOnly := &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kind}, ObjectMeta: *objectMeta.DeepCopy()}
	return []runtime.Object{object, metadataOnly}
}

func objectKey(tracker k8stesting.ObjectTracker, gvr schema.GroupVersionResource, namespace, name, resourceVersion string) string {
	return fmt.Sprintf("%p %s %s/%s %s", tracker, gvr, namespace, name, resourceVersion)
}

// forbid makes the API server forbid listing the resource of the dynamic client.
func (c *fakeCluster) forbid(gvr schema.GroupVersionResource) {
	c.dynamic.PrependReactor("list", gvr.Resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetResource() != gvr {
			return false, nil, nil
		}
		return true, nil, apierrors.NewForbidden(gvr.GroupResource(), "", errors.New("RBAC: access denied"))
	})
}

// failLists makes every list of the resource of the typed client fail, so that its informer never syncs.
func (c *fakeCluster) failLists(gvr schema.GroupVersionResource) {
	c.client.PrependReactor("list", gvr.Resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetResource() != gvr {
			return false, nil, nil
		}
		return true, nil, apierrors.NewInternalError(errors.New("the API server failed"))
	})
}

// discover makes API discovery report the resource, after the resources that it already reports.
func (c *fakeCluster) discover(groupVersion string, resource metav1.APIResource) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.discovered = append(c.discovered, discoveredResource{groupVersion, resource})
}

// failGroupVersions makes discovery fail for the group versions only, like an unavailable aggregated API.
func (c *fakeCluster) failGroupVersions(groupVersions []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failedGroupVersions = groupVersions
}

func (c *fakeCluster) failDiscovery(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.discoveryErr = err
}

// serverGroupsAndResources serves API discovery: the discovered resources, or the error that a test sets.
func (c *fakeCluster) serverGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	c.mu.Lock()
	discovered, err, failed := slices.Clone(c.discovered), c.discoveryErr, slices.Clone(c.failedGroupVersions)
	c.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	var partialErr error
	if len(failed) > 0 {
		groupErr := &discovery.ErrGroupDiscoveryFailed{Groups: map[schema.GroupVersion]error{}}
		for _, groupVersion := range failed {
			gv, err := schema.ParseGroupVersion(groupVersion)
			if err != nil {
				return nil, nil, err
			}
			groupErr.Groups[gv] = errors.New("the aggregated API is unavailable")
		}
		partialErr = groupErr
	}

	var groups []*metav1.APIGroup
	var lists []*metav1.APIResourceList
	for _, d := range discovered {
		i := slices.IndexFunc(lists, func(l *metav1.APIResourceList) bool { return l.GroupVersion == d.groupVersion })
		if i < 0 {
			lists = append(lists, &metav1.APIResourceList{GroupVersion: d.groupVersion})
			i = len(lists) - 1
		}
		lists[i].APIResources = append(lists[i].APIResources, d.resource)
	}
	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			return nil, nil, err
		}
		i := slices.IndexFunc(groups, func(g *metav1.APIGroup) bool { return g.Name == gv.Group })
		if i < 0 {
			groups = append(groups, &metav1.APIGroup{Name: gv.Group})
			i = len(groups) - 1
		}
		versionForDiscovery := metav1.GroupVersionForDiscovery{GroupVersion: list.GroupVersion, Version: gv.Version}
		groups[i].Versions = append(groups[i].Versions, versionForDiscovery)
		if groups[i].PreferredVersion.Version == "" {
			groups[i].PreferredVersion = versionForDiscovery
		}
	}
	// The API groups still list a failed group version, as the API server does, but its resources are missing.
	lists = slices.DeleteFunc(lists, func(l *metav1.APIResourceList) bool { return slices.Contains(failed, l.GroupVersion) })
	return groups, lists, partialErr
}

// tracker returns the object tracker of the fake client that serves the object, and the resource of the object.
func (c *fakeCluster) tracker(t testing.TB, object runtime.Object) (k8stesting.ObjectTracker, schema.GroupVersionResource) {
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

// serveWatches makes the watches of the fake client behave like the API server, which the fake clients do not: a
// watch applies its label and field selectors, reports an object that starts matching them as added and one that
// stops matching them as deleted, and reports a deleted object with a new resource version.
func (c *fakeCluster) serveWatches(f *k8stesting.Fake, tracker k8stesting.ObjectTracker) {
	f.PrependWatchReactor("*", func(action k8stesting.Action) (bool, watch.Interface, error) {
		restrictions := action.(k8stesting.WatchAction).GetWatchRestrictions()
		gvr := action.GetResource()
		w, err := tracker.Watch(gvr, action.GetNamespace())
		if err != nil {
			return true, nil, err
		}
		return true, watch.Filter(w, func(event watch.Event) (watch.Event, bool) {
			matchesNow := matches(event.Object, restrictions.Labels, restrictions.Fields)
			switch event.Type {
			case watch.Modified:
				matchedBefore := matchesNow
				if accessor, err := meta.Accessor(event.Object); err == nil {
					c.mu.Lock()
					previous, found := c.previous[objectKey(tracker, gvr, accessor.GetNamespace(), accessor.GetName(), accessor.GetResourceVersion())]
					c.mu.Unlock()
					if found {
						matchedBefore = matches(previous, restrictions.Labels, restrictions.Fields)
					}
				}
				switch {
				case matchedBefore && !matchesNow:
					event.Type = watch.Deleted
					return event, true
				case !matchedBefore && matchesNow:
					event.Type = watch.Added
				}
			case watch.Deleted:
				event.Object = withNextResourceVersion(event.Object)
			}
			return event, matchesNow
		}), nil
	})
}

// serveFieldSelectors makes the lists of the fake client apply field selectors, as the unassigned pod informer lists
// the pods with no node.
func serveFieldSelectors(f *k8stesting.Fake, tracker k8stesting.ObjectTracker) {
	f.PrependReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list := action.(k8stesting.ListActionImpl)
		restrictions := list.GetListRestrictions()
		if restrictions.Fields == nil || restrictions.Fields.Empty() {
			return false, nil, nil
		}
		objects, err := tracker.List(list.GetResource(), list.GetKind(), list.GetNamespace())
		if err != nil {
			return true, nil, err
		}
		items, err := meta.ExtractList(objects)
		if err != nil {
			return true, nil, err
		}
		items = slices.DeleteFunc(items, func(o runtime.Object) bool { return !matches(o, restrictions.Labels, restrictions.Fields) })
		return true, objects, meta.SetList(objects, items)
	})
}

func matches(object runtime.Object, labelSelector labels.Selector, fieldSelector fields.Selector) bool {
	accessor, err := meta.Accessor(object)
	if err != nil {
		return true
	}
	if labelSelector != nil && !labelSelector.Matches(labels.Set(accessor.GetLabels())) {
		return false
	}
	if fieldSelector == nil {
		return true
	}
	set := fields.Set{"metadata.name": accessor.GetName(), "metadata.namespace": accessor.GetNamespace()}
	if pod, ok := object.(*corev1.Pod); ok {
		set["spec.nodeName"], set["status.phase"] = pod.Spec.NodeName, string(pod.Status.Phase)
	}
	return fieldSelector.Matches(set)
}

// withNextResourceVersion returns a copy of the object with the next resource version. The check skips an object whose
// resource version it has already sent, so a deletion must have a new one, as the API server reports.
func withNextResourceVersion(object runtime.Object) runtime.Object {
	object = object.DeepCopyObject()
	accessor, err := meta.Accessor(object)
	if err != nil {
		return object
	}
	accessor.SetResourceVersion(nextResourceVersion(accessor.GetResourceVersion()))
	return object
}

func nextResourceVersion(resourceVersion string) string {
	v, err := strconv.Atoi(resourceVersion)
	if err != nil {
		return resourceVersion + "1"
	}
	return strconv.Itoa(v + 1)
}

func namespaceOf(t testing.TB, object runtime.Object) string {
	accessor, err := meta.Accessor(object)
	require.NoError(t, err)
	return accessor.GetNamespace()
}
