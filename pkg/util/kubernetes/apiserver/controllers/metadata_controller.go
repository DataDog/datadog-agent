// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package controllers

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	agentcache "github.com/DataDog/datadog-agent/pkg/util/cache"
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
type metadataController struct {
	wmeta workloadmeta.Component

	endpointsLister       corelisters.EndpointsLister
	endpointsListerSynced cache.InformerSynced

	store *metaBundleStore

	// Endpoints that need to be added to services mapping.
	queue workqueue.RateLimitingInterface
}

// newMetadataController returns a new metadata controller
func newMetadataController(endpointsInformer coreinformers.EndpointsInformer, wmeta workloadmeta.Component) *metadataController {
	m := &metadataController{
		wmeta: wmeta,
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "endpoints"),
	}

	if _, err := endpointsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    m.addEndpoints,
		UpdateFunc: m.updateEndpoints,
		DeleteFunc: m.deleteEndpoints,
	}); err != nil {
		log.Errorf("error adding event handler to node informer: %f", err)
	}
	m.endpointsLister = endpointsInformer.Lister()
	m.endpointsListerSynced = endpointsInformer.Informer().HasSynced

	m.store = globalMetaBundleStore // default to global store

	return m
}

// run starts the metadata controller reconciler loop
func (m *metadataController) run(stopCh <-chan struct{}) {
	defer m.queue.ShutDown()

	log.Infof("Starting metadata controller")
	defer log.Infof("Stopping metadata controller")

	if !cache.WaitForCacheSync(stopCh, m.endpointsListerSynced) {
		return
	}

	wmetaFilterParams := workloadmeta.FilterParams{
		Kinds:     []workloadmeta.Kind{workloadmeta.KindKubernetesNode},
		Source:    workloadmeta.SourceAll,
		EventType: workloadmeta.EventTypeAll,
	}

	wmetaEventsCh := m.wmeta.Subscribe(
		"metadata-controller",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(&wmetaFilterParams),
	)
	defer m.wmeta.Unsubscribe(wmetaEventsCh)

	go m.processWorkloadmetaNodeEvents(wmetaEventsCh)
	go wait.Until(m.worker, time.Second, stopCh)
	<-stopCh
}

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

	err := m.syncEndpoints(key.(string))
	if err != nil {
		log.Debugf("Error syncing endpoints %v: %v", key, err)
	}

	return true
}

func (m *metadataController) processWorkloadmetaNodeEvents(wmetaEventsCh chan workloadmeta.EventBundle) {
	for eventBundle := range wmetaEventsCh {
		eventBundle.Acknowledge()

		for _, event := range eventBundle.Events {
			node := event.Entity.(*workloadmeta.KubernetesNode)

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

func (m *metadataController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	m.queue.Add(key)
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
		err = m.deleteMappedEndpoints(namespace, name)
	case err != nil:
		log.Debugf("Unable to retrieve endpoints %v from store: %v", key, err)
	default:
		err = m.mapEndpoints(endpoints)
	}
	return err
}

// mapEndpoints matches pods to services via endpoint TargetRef objects. It supports Kubernetes 1.4+.
func (m *metadataController) mapEndpoints(endpoints *corev1.Endpoints) error {
	nodeToPods := make(map[string]map[string]sets.Set[string])

	// Loop over the subsets to create a mapping of nodes to pods running on the node.
	for _, subset := range endpoints.Subsets {
		for _, address := range subset.Addresses {
			if address.TargetRef == nil {
				// Endpoints are also used by the control plane as resource locks for leader election.
				// These endpoints will not have a TargetRef and can be ignored.
				log.Tracef("No TargetRef for endpoints %s/%s, skipping", endpoints.Namespace, endpoints.Name)
				continue
			}
			if address.TargetRef.Kind != "Pod" {
				continue
			}
			namespace := address.TargetRef.Namespace
			podName := address.TargetRef.Name
			if podName == "" || namespace == "" {
				log.Tracef("Incomplete reference for object %s on service %s/%s, skipping",
					address.TargetRef.UID, endpoints.Namespace, endpoints.Name)
				continue
			}

			// TODO: Kubernetes 1.3.x does not include `NodeName`
			if address.NodeName == nil {
				continue
			}

			nodeName := *address.NodeName

			if _, ok := nodeToPods[nodeName]; !ok {
				nodeToPods[nodeName] = make(map[string]sets.Set[string])
			}
			if _, ok := nodeToPods[nodeName][namespace]; !ok {
				nodeToPods[nodeName][namespace] = sets.New[string]()
			}
			nodeToPods[nodeName][namespace].Insert(podName)
		}
	}

	svc := endpoints.Name
	namespace := endpoints.Namespace
	for nodeName, ns := range nodeToPods {
		metaBundle := m.store.getCopyOrNew(nodeName)
		metaBundle.Services.Delete(namespace, svc) // cleanup pods deleted from the service
		for _, pods := range ns {
			for podName := range pods {
				metaBundle.Services.Set(namespace, podName, svc)
			}
		}

		m.store.set(nodeName, metaBundle)
	}

	return nil
}

func (m *metadataController) deleteMappedEndpoints(namespace, svc string) error {
	nodes := m.wmeta.ListKubernetesNodes()

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

// GetPodMetadataNames is used when the API endpoint of the DCA to get the metadata of a pod is hit.
func GetPodMetadataNames(nodeName, ns, podName string) ([]string, error) {
	cacheKey := agentcache.BuildAgentKey(apiserver.MetadataMapperCachePrefix, nodeName)
	metaBundleInterface, found := agentcache.Cache.Get(cacheKey)
	if !found {
		log.Tracef("no metadata was found for the pod %s on node %s", podName, nodeName)
		return nil, nil
	}

	metaBundle, ok := metaBundleInterface.(*apiserver.MetadataMapperBundle)
	if !ok {
		return nil, fmt.Errorf("invalid cache format for the cacheKey: %s", cacheKey)
	}
	// The list of metadata collected in the metaBundle is extensible and is handled here.
	// If new cluster level tags need to be collected by the agent, only this needs to be modified.
	serviceList, foundServices := metaBundle.ServicesForPod(ns, podName)
	if !foundServices {
		log.Tracef("no cached services list found for the pod %s on the node %s", podName, nodeName)
		return nil, nil
	}
	log.Tracef("CacheKey: %s, with %d services", cacheKey, len(serviceList))
	var metaList []string
	for _, s := range serviceList {
		metaList = append(metaList, fmt.Sprintf("kube_service:%s", s))
	}
	return metaList, nil
}
