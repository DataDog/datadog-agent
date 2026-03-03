// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package controllers

import (
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	corelisters "k8s.io/client-go/listers/core/v1"
	disclisters "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// metadataController is responsible for synchronizing objects from the Kubernetes
// apiserver to build and cache cluster metadata (like service tags) for each node.
//
// The controller takes care to garbage collect any data while processing updates/deletes
// so that the cache does not contain data for deleted pods/services.
//
// This controller is used by the Datadog Cluster Agent and supports Kubernetes 1.4+.

const kubernetesServiceNameLabel = "kubernetes.io/service-name"

type metadataController struct {
	wmeta workloadmeta.Component

	endpointsLister     corelisters.EndpointsLister
	endpointSliceLister disclisters.EndpointSliceLister
	listerSynced        cache.InformerSynced
	useEndpointSlices   bool

	// Cache to map endpointSlices to their corresponding services.
	// - Allows cleanup of metadata when an endpointSlice is deleted since endpointSlices
	//   do not directly map to services.
	// - Prevents redundant processing by tracking the resource version of endpointSlices.
	sliceServiceCache map[string]map[string]sliceCacheEntry
	sliceCacheLock    sync.RWMutex

	// Stores the mapping of services to nodes.
	// - Tracks nodes associated with services to detect when all pods on a node
	//   are removed from a service.
	serviceToNodes     map[string]sets.Set[string]
	serviceToNodesLock sync.RWMutex

	store *metaBundleStore

	// Endpoints that need to be added to services mapping.
	queue workqueue.TypedRateLimitingInterface[string]
}

// sliceCacheEntry represents the service and resource version of an endpointSlice,
// enabling metadata cleanup and change detection.
type sliceCacheEntry struct {
	serviceName     string
	resourceVersion string
}

// newMetadataController returns a new metadata controller
func newMetadataController(informerFactory informers.SharedInformerFactory, wmeta workloadmeta.Component, useEndpointSlices bool) *metadataController {
	m := &metadataController{
		wmeta: wmeta,
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "endpoints"},
		),
		useEndpointSlices: useEndpointSlices,
	}

	if useEndpointSlices {
		log.Info("Initializing metadata controller with endpointSlices")
		endpointSliceInformer := informerFactory.Discovery().V1().EndpointSlices()
		if _, err := endpointSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    m.addEndpointSlice,
			UpdateFunc: m.updateEndpointSlice,
			DeleteFunc: m.deleteEndpointSlice,
		}); err != nil {
			log.Errorf("error adding event handler to endpointSlice informer: %v", err)
		}
		m.sliceServiceCache = make(map[string]map[string]sliceCacheEntry)
		m.endpointSliceLister = endpointSliceInformer.Lister()
		m.listerSynced = endpointSliceInformer.Informer().HasSynced
	} else {
		log.Info("Initializing metadata controller with endpoints")
		endpointInformer := informerFactory.Core().V1().Endpoints()
		if _, err := endpointInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    m.addEndpoints,
			UpdateFunc: m.updateEndpoints,
			DeleteFunc: m.deleteEndpoints,
		}); err != nil {
			log.Errorf("error adding event handler to node informer: %f", err)
		}
		m.endpointsLister = endpointInformer.Lister()
		m.listerSynced = endpointInformer.Informer().HasSynced
	}

	m.serviceToNodes = make(map[string]sets.Set[string])
	m.store = globalMetaBundleStore // default to global store

	return m
}

// run starts the metadata controller reconciler loop
func (m *metadataController) run(stopCh <-chan struct{}) {
	defer m.queue.ShutDown()

	log.Infof("Starting metadata controller")
	defer log.Infof("Stopping metadata controller")

	if !cache.WaitForCacheSync(stopCh, m.listerSynced) {
		return
	}

	filter := workloadmeta.NewFilterBuilder().AddKindWithEntityFilter(
		workloadmeta.KindKubernetesMetadata,
		func(entity workloadmeta.Entity) bool {
			metadata := entity.(*workloadmeta.KubernetesMetadata)
			return workloadmeta.IsNodeMetadata(metadata)
		},
	).Build()

	wmetaEventsCh := m.wmeta.Subscribe(
		"metadata-controller",
		workloadmeta.NormalPriority,
		filter,
	)
	defer m.wmeta.Unsubscribe(wmetaEventsCh)

	go m.processWorkloadmetaNodeMetadataEvents(wmetaEventsCh)
	go wait.Until(m.worker, time.Second, stopCh)
	<-stopCh
}

// worker processes items in the work queue.
func (m *metadataController) worker() {
	for m.processNextWorkItem() {
	}
}

func (m *metadataController) processNextWorkItem() bool {
	key, quit := m.queue.Get()
	if quit {
		return false
	}
	defer m.queue.Done(key)

	err := m.sync(key)
	if err != nil {
		log.Debugf("Error syncing endpoints %v: %v", key, err)
	}

	return true
}

func (m *metadataController) processWorkloadmetaNodeMetadataEvents(wmetaEventsCh chan workloadmeta.EventBundle) {
	for eventBundle := range wmetaEventsCh {
		eventBundle.Acknowledge()

		for _, event := range eventBundle.Events {
			node := event.Entity.(*workloadmeta.KubernetesMetadata)

			switch event.Type {
			case workloadmeta.EventTypeSet:
				m.addNode(node.Name)
			case workloadmeta.EventTypeUnset:
				m.deleteNode(node.Name)
			default:
				log.Warnf("Unknown event type %v", event.Type)
			}
		}
	}
}

func (m *metadataController) addNode(name string) {
	bundle := m.store.getCopyOrNew(name)
	m.store.set(name, bundle)
	log.Debugf("Detected node %s", name)
}

func (m *metadataController) deleteNode(name string) {
	m.store.delete(name)
	log.Debugf("Forgot node %s", name)
}

func (m *metadataController) addEndpoints(obj interface{}) {
	endpoints, ok := obj.(*corev1.Endpoints)
	if !ok {
		return
	}
	log.Debugf("Adding endpoints %s/%s", endpoints.Namespace, endpoints.Name)
	m.enqueue(obj)
}

func (m *metadataController) updateEndpoints(_, cur interface{}) {
	newEndpoints, ok := cur.(*corev1.Endpoints)
	if !ok {
		return
	}
	log.Tracef("Updating endpoints %s/%s", newEndpoints.Namespace, newEndpoints.Name)
	m.enqueue(cur)
}

func (m *metadataController) deleteEndpoints(obj interface{}) {
	endpoints, ok := obj.(*corev1.Endpoints)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Debugf("Couldn't get object from tombstone %#v", obj)
			return
		}
		endpoints, ok = tombstone.Obj.(*corev1.Endpoints)
		if !ok {
			log.Debugf("Tombstone contained object that is not an endpoint %#v", obj)
			return
		}
	}
	log.Debugf("Deleting endpoints %s/%s", endpoints.Namespace, endpoints.Name)
	m.enqueue(obj)
}

func (m *metadataController) addEndpointSlice(obj interface{}) {
	endpointSlice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		return
	}
	log.Debugf("Adding endpointSlice %s/%s", endpointSlice.Namespace, endpointSlice.Name)
	m.enqueue(obj)
}

func (m *metadataController) updateEndpointSlice(_, cur interface{}) {
	newEndpointSlice, ok := cur.(*discv1.EndpointSlice)
	if !ok {
		return
	}
	log.Tracef("Updating endpointSlice %s/%s", newEndpointSlice.Namespace, newEndpointSlice.Name)
	m.enqueue(cur)
}

func (m *metadataController) deleteEndpointSlice(obj interface{}) {
	endpointSlice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Debugf("Couldn't get object from tombstone %#v", obj)
			return
		}
		endpointSlice, ok = tombstone.Obj.(*discv1.EndpointSlice)
		if !ok {
			log.Debugf("Tombstone contained object that is not an endpointSlice %#v", obj)
			return
		}
	}
	log.Debugf("Deleting endpointSlice %s/%s", endpointSlice.Namespace, endpointSlice.Name)
	m.enqueue(obj)
}

func (m *metadataController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	m.queue.Add(key)
}

func (m *metadataController) sync(key string) error {
	if m.useEndpointSlices {
		return m.syncEndpointSlices(key)
	}
	return m.syncEndpoints(key)
}

func (m *metadataController) syncEndpoints(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	endpoints, err := m.endpointsLister.Endpoints(namespace).Get(name)
	switch {
	case errors.IsNotFound(err):
		// Endpoints absence in store means watcher caught the deletion, ensure metadata map is cleaned.
		log.Tracef("Endpoints has been deleted %v. Attempting to cleanup metadata map", key)
		m.serviceToNodesLock.Lock()
		delete(m.serviceToNodes, name)
		m.serviceToNodesLock.Unlock()
		err = m.deleteService(namespace, name)
	case err != nil:
		log.Debugf("Unable to retrieve endpoints %v from store: %v", key, err)
	default:
		m.mapEndpoints(endpoints)
	}
	return err
}

func (m *metadataController) syncEndpointSlices(key string) error {
	namespace, sliceName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	endpointSlice, err := m.endpointSliceLister.EndpointSlices(namespace).Get(sliceName)
	switch {
	case errors.IsNotFound(err):
		// Endpoints absence in store means watcher caught the deletion, ensure metadata map is cleaned.
		log.Tracef("EndpointSlice has been deleted %v. Attempting to cleanup metadata map", key)
		err = m.cleanupEndpointSlice(namespace, sliceName)
	case err != nil:
		log.Debugf("Unable to retrieve endpointSlice %v from store: %v", key, err)
	default:
		err = m.mapEndpointSlice(endpointSlice)
	}
	return err
}

// mapEndpoints matches pods to services via endpoint TargetRef objects. It supports Kubernetes 1.4+.
func (m *metadataController) mapEndpoints(endpoints *corev1.Endpoints) {
	nodeToPods := make(map[string]map[string]sets.Set[string])
	affectedNodes := sets.New[string]()

	// Loop over the subsets to create a mapping of nodes to pods running on the node.
	for _, subset := range endpoints.Subsets {
		m.mapEndpointAddresses(subset.Addresses, endpoints.Namespace, endpoints.Name, nodeToPods, affectedNodes)
		m.mapEndpointAddresses(subset.NotReadyAddresses, endpoints.Namespace, endpoints.Name, nodeToPods, affectedNodes)
	}

	m.cleanupStaleNodes(endpoints.Namespace, endpoints.Name, affectedNodes)
	m.updateNodeMetadata(endpoints.Namespace, endpoints.Name, nodeToPods)
}

func (m *metadataController) mapEndpointAddresses(address []corev1.EndpointAddress, ns, name string,
	nodeToPods map[string]map[string]sets.Set[string], affectedNodes sets.Set[string],
) {
	for _, address := range address {
		if address.TargetRef == nil {
			// Endpoints are also used by the control plane as resource locks for leader election.
			// These endpoints will not have a TargetRef and can be ignored.
			log.Tracef("No TargetRef for endpoints %s/%s, skipping", ns, name)
			continue
		}
		if address.TargetRef.Kind != "Pod" {
			continue
		}
		namespace := address.TargetRef.Namespace
		podName := address.TargetRef.Name
		if podName == "" || namespace == "" {
			log.Tracef("Incomplete reference for object %s on service %s/%s, skipping",
				address.TargetRef.UID, ns, name)
			continue
		}

		// TODO: Kubernetes 1.3.x does not include `NodeName`
		if address.NodeName == nil {
			continue
		}

		nodeName := *address.NodeName

		affectedNodes.Insert(nodeName)

		if _, ok := nodeToPods[nodeName]; !ok {
			nodeToPods[nodeName] = make(map[string]sets.Set[string])
		}
		if _, ok := nodeToPods[nodeName][namespace]; !ok {
			nodeToPods[nodeName][namespace] = sets.New[string]()
		}
		nodeToPods[nodeName][namespace].Insert(podName)
	}
}

// mapEndpointSlice matches pods to services via endpoint TargetRef objects. It supports Kubernetes 1.19+.
func (m *metadataController) mapEndpointSlice(endpointSlice *discv1.EndpointSlice) error {
	svc, svcExists := endpointSlice.Labels[kubernetesServiceNameLabel]
	if !svcExists || svc == "" {
		log.Tracef("EndpointSlice %s/%s missing %s label; skipping", endpointSlice.Namespace, endpointSlice.Name, kubernetesServiceNameLabel)
		return nil
	}

	m.sliceCacheLock.RLock()
	cachedEntry, exists := m.sliceServiceCache[endpointSlice.Namespace][endpointSlice.Name]
	m.sliceCacheLock.RUnlock()

	// Skip processing if the resource version hasn't changed
	if exists && cachedEntry.resourceVersion == endpointSlice.ResourceVersion {
		log.Tracef("No change detected for EndpointSlice %s/%s; skipping", endpointSlice.Namespace, endpointSlice.Name)
		return nil
	}

	m.sliceCacheLock.Lock()
	if _, ok := m.sliceServiceCache[endpointSlice.Namespace]; !ok {
		m.sliceServiceCache[endpointSlice.Namespace] = make(map[string]sliceCacheEntry)
	}
	m.sliceServiceCache[endpointSlice.Namespace][endpointSlice.Name] = sliceCacheEntry{
		serviceName:     svc,
		resourceVersion: endpointSlice.ResourceVersion,
	}
	m.sliceCacheLock.Unlock()

	allSlices, err := m.endpointSliceLister.EndpointSlices(endpointSlice.Namespace).List(labels.Set{kubernetesServiceNameLabel: svc}.AsSelector())
	if err != nil {
		return fmt.Errorf("failed to fetch EndpointSlices for Service %s/%s: %w", endpointSlice.Namespace, svc, err)
	}

	nodeToPods, affectedNodes := m.buildNodeToPodsMapping(allSlices)
	m.cleanupStaleNodes(endpointSlice.Namespace, svc, affectedNodes)
	m.updateNodeMetadata(endpointSlice.Namespace, svc, nodeToPods)

	return nil
}

// cleanupEndpointSlice removes metadata associated with a deleted EndpointSlice.
func (m *metadataController) cleanupEndpointSlice(namespace, sliceName string) error {
	serviceName := m.getCachedServiceName(namespace, sliceName)
	if serviceName == "" {
		return fmt.Errorf("no service found for EndpointSlice %s/%s in cache", namespace, sliceName)
	}

	m.sliceCacheLock.Lock()
	defer m.sliceCacheLock.Unlock()

	slices, ok := m.sliceServiceCache[namespace]
	if ok {
		delete(slices, sliceName)
		if len(slices) == 0 {
			delete(m.sliceServiceCache, namespace)
		}
	}

	remainingSlices, err := m.endpointSliceLister.EndpointSlices(namespace).List(labels.Set{kubernetesServiceNameLabel: serviceName}.AsSelector())
	if err != nil {
		return fmt.Errorf("failed to fetch remaining EndpointSlices for Service %s/%s: %w", namespace, serviceName, err)
	}

	nodeToPods, affectedNodes := m.buildNodeToPodsMapping(remainingSlices)
	m.cleanupStaleNodes(namespace, serviceName, affectedNodes)
	m.updateNodeMetadata(namespace, serviceName, nodeToPods)

	if len(remainingSlices) == 0 {
		m.serviceToNodesLock.Lock()
		delete(m.serviceToNodes, serviceName)
		m.serviceToNodesLock.Unlock()

		err := m.deleteService(namespace, serviceName)
		if err != nil {
			return fmt.Errorf("failed to delete service metadata for Service %s/%s: %w", namespace, serviceName, err)
		}
	}

	return nil
}

// buildNodeToPodsMapping builds the node-to-pods mapping from a list of EndpointSlices.
func (m *metadataController) buildNodeToPodsMapping(slices []*discv1.EndpointSlice) (map[string]map[string]sets.Set[string], sets.Set[string]) {
	nodeToPods := make(map[string]map[string]sets.Set[string])
	affectedNodes := sets.New[string]()

	for _, slice := range slices {
		for _, endpoint := range slice.Endpoints {
			if endpoint.TargetRef == nil || endpoint.TargetRef.Kind != "Pod" {
				continue
			}

			podNamespace := endpoint.TargetRef.Namespace
			podName := endpoint.TargetRef.Name
			if podNamespace == "" || podName == "" {
				continue
			}

			nodeName := ""
			if endpoint.NodeName != nil {
				nodeName = *endpoint.NodeName
			}

			affectedNodes.Insert(nodeName)

			if _, ok := nodeToPods[nodeName]; !ok {
				nodeToPods[nodeName] = make(map[string]sets.Set[string])
			}
			if _, ok := nodeToPods[nodeName][podNamespace]; !ok {
				nodeToPods[nodeName][podNamespace] = sets.New[string]()
			}
			nodeToPods[nodeName][podNamespace].Insert(podName)
		}
	}

	return nodeToPods, affectedNodes
}

// cleanupStaleNodes removes stale nodes for a given service.
func (m *metadataController) cleanupStaleNodes(namespace, serviceName string, affectedNodes sets.Set[string]) {
	m.serviceToNodesLock.Lock()
	defer m.serviceToNodesLock.Unlock()

	staleNodes := m.serviceToNodes[serviceName].Difference(affectedNodes)
	for nodeName := range staleNodes {
		log.Tracef("Cleaning up stale service metadata for Node %s and Service %s/%s", nodeName, namespace, serviceName)
		metaBundle := m.store.getCopyOrNew(nodeName)
		metaBundle.Services.Delete(namespace, serviceName)
		m.store.set(nodeName, metaBundle)
	}

	m.serviceToNodes[serviceName] = affectedNodes
}

// updateNodeMetadata updates metadata for nodes based on node-to-pods mapping.
func (m *metadataController) updateNodeMetadata(namespace, serviceName string, nodeToPods map[string]map[string]sets.Set[string]) {
	for nodeName, podsByNamespace := range nodeToPods {
		metaBundle := m.store.getCopyOrNew(nodeName)
		metaBundle.Services.Delete(namespace, serviceName) // Cleanup old metadata
		for podNamespace, pods := range podsByNamespace {
			for podName := range pods {
				metaBundle.Services.Set(podNamespace, podName, serviceName)
			}
		}
		m.store.set(nodeName, metaBundle)
	}
}

// getCachedServiceName retrieves the cached service name for a given EndpointSlice.
func (m *metadataController) getCachedServiceName(namespace, sliceName string) string {
	m.sliceCacheLock.RLock()
	defer m.sliceCacheLock.RUnlock()

	if slices, ok := m.sliceServiceCache[namespace]; ok {
		if cachedEntry, ok := slices[sliceName]; ok {
			return cachedEntry.serviceName
		}
	}
	return ""
}

// deleteService removes a service's metadata from all nodes.
func (m *metadataController) deleteService(namespace, svc string) error {
	nodes := m.wmeta.ListKubernetesMetadata(workloadmeta.IsNodeMetadata)

	// Delete the service from the metadata bundle for each node.
	for _, node := range nodes {
		oldBundle, ok := m.store.get(node.Name)
		if !ok {
			// Nothing to delete.
			continue
		}
		newMetaBundle := apiserver.NewMetadataMapperBundle()
		newMetaBundle.DeepCopy(oldBundle)
		newMetaBundle.Services.Delete(namespace, svc)

		m.store.set(node.Name, newMetaBundle)
	}
	return nil
}
