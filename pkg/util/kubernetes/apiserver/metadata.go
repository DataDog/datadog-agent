// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	utilcache "github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	maxRetries = 15
	numWorkers = 3
)

// MetadataController is responsible for synchronizing Endpoints objects from the Kubernetes
// apiserver to build and cache cluster metadata (like service tags) for each node.
// This controller only supports Kubernetes 1.4+.
type MetadataController struct {
	nodeLister       corelisters.NodeLister
	nodeListerSynced cache.InformerSynced

	endpointsLister       corelisters.EndpointsLister
	endpointsListerSynced cache.InformerSynced

	metadataMapExpire time.Duration // must be greater than the endpoints `resyncPeriod`

	// Endpoints that need to be added to services mapping.
	queue workqueue.RateLimitingInterface

	// used in unit tests for keys that are removed from the queue
	endpoints chan interface{}
}

func NewMetadataController(nodeInformer coreinformers.NodeInformer, endpointsInformer coreinformers.EndpointsInformer) *MetadataController {
	resyncPeriod := time.Duration(config.Datadog.GetInt64("kubernetes_metadata_resync_period")) * time.Second

	m := &MetadataController{
		metadataMapExpire: 2 * resyncPeriod,
		queue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "metadata"),
	}
	m.nodeLister = nodeInformer.Lister()
	m.nodeListerSynced = nodeInformer.Informer().HasSynced

	endpointsInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    m.addEndpoints,
			UpdateFunc: m.updateEndpoints,
			DeleteFunc: m.deleteEndpoints,
		},
		resyncPeriod, // delay for re-listing endpoints
	)
	m.endpointsLister = endpointsInformer.Lister()
	m.endpointsListerSynced = endpointsInformer.Informer().HasSynced

	return m
}

func (m *MetadataController) Run(stopCh <-chan struct{}) {
	defer m.queue.ShutDown()

	log.Infof("Starting metadata controller")
	defer log.Infof("Stopping metadata controller")

	if !cache.WaitForCacheSync(stopCh, m.nodeListerSynced, m.endpointsListerSynced) {
		return
	}

	for i := 0; i < numWorkers; i++ {
		go wait.Until(m.worker, time.Second, stopCh)
	}

	<-stopCh
}

func (m *MetadataController) worker() {
	for m.processNextWorkItem() {
	}
}

func (m *MetadataController) processNextWorkItem() bool {
	key, quit := m.queue.Get()
	if quit {
		return false
	}
	defer m.queue.Done(key)

	err := m.syncEndpoints(key.(string))
	if err == nil {
		m.queue.Forget(key)
		if m.endpoints != nil {
			m.endpoints <- key
		}
		return true
	}

	if m.queue.NumRequeues(key) < maxRetries {
		log.Debugf("Error syncing endpoints %v: %v", key, err)
		m.queue.AddRateLimited(key)
		return true
	}

	log.Debugf("Dropping endpoints %q out of the queue: %v", key, err)
	m.queue.Forget(key)
	if m.endpoints != nil {
		m.endpoints <- key
	}

	return true
}

func (m *MetadataController) addEndpoints(obj interface{}) {
	endpoints := obj.(*corev1.Endpoints)
	log.Tracef("Adding endpoints %s/%s", endpoints.Namespace, endpoints.Name)
	m.enqueue(obj)
}

func (m *MetadataController) updateEndpoints(_, new interface{}) {
	newEndpoints := new.(*corev1.Endpoints)
	log.Tracef("Updating endpoints %s/%s", newEndpoints.Namespace, newEndpoints.Name)
	m.enqueue(new)
}

func (m *MetadataController) deleteEndpoints(obj interface{}) {
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
	log.Tracef("Deleting endpoints %s/%s", endpoints.Namespace, endpoints.Name)
	m.enqueue(obj)
}

func (m *MetadataController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	m.queue.Add(key)
}

func (m *MetadataController) syncEndpoints(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	endpoints, err := m.endpointsLister.Endpoints(namespace).Get(name)
	switch {
	case errors.IsNotFound(err):
		// Endpoints absence in store means watcher caught the deletion, ensure metadata map is cleaned.
		log.Tracef("Endpoints has been deleted %v. Attempting to cleanup metadata map", key)
		err = m.mapDeletedEndpoints(namespace, name)
	case err != nil:
		log.Debugf("Unable to retrieve endpoints %v from store: %v", key, err)
	default:
		err = m.mapEndpoints(endpoints)
	}
	return err
}

func (m *MetadataController) mapEndpoints(endpoints *corev1.Endpoints) error {
	nodeToPods := make(map[string]map[string]string)

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
				log.Debugf("Incomplete reference for object %s/%s on service %s/%s, skipping",
					address.TargetRef.UID, endpoints.Namespace, endpoints.Name)
				continue
			}

			// TODO: Kubernetes 1.3.x does not include `NodeName`
			if address.NodeName == nil {
				continue
			}

			nodeName := *address.NodeName

			if _, ok := nodeToPods[nodeName]; !ok {
				nodeToPods[nodeName] = make(map[string]string)
			}
			nodeToPods[nodeName][namespace] = podName
		}
	}

	svc := endpoints.Name
	for nodeName, pods := range nodeToPods {
		metaBundle, err := getMetadataMapBundle(nodeName)
		if err != nil {
			log.Tracef("Could not get metadata for node %s", nodeName)
			metaBundle = newMetadataMapperBundle()
		}

		metaBundle.m.Lock()
		for namespace, podName := range pods {
			metaBundle.Services.Set(namespace, podName, svc)
		}
		metaBundle.m.Unlock()

		cacheKey := utilcache.BuildAgentKey(metadataMapperCachePrefix, nodeName)

		utilcache.Cache.Set(cacheKey, metaBundle, m.metadataMapExpire)
	}

	return nil
}

func (m *MetadataController) mapDeletedEndpoints(namespace, svc string) error {
	nodes, err := m.nodeLister.List(labels.Everything()) // list all nodes
	if err != nil {
		return err
	}

	// Delete the service from the metadata bundle for each node.
	for _, node := range nodes {
		metaBundle, err := getMetadataMapBundle(node.Name)
		if err != nil {
			// Nothing to delete.
			continue
		}

		metaBundle.m.Lock()
		metaBundle.Services.Delete(namespace, svc)
		metaBundle.m.Unlock()

		cacheKey := utilcache.BuildAgentKey(metadataMapperCachePrefix, node.Name)

		if len(metaBundle.Services) == 0 {
			utilcache.Cache.Delete(cacheKey)
			continue
		}
		utilcache.Cache.Set(cacheKey, metaBundle, m.metadataMapExpire)
	}
	return nil
}

// GetPodMetadataNames is used when the API endpoint of the DCA to get the metadata of a pod is hit.
func GetPodMetadataNames(nodeName, ns, podName string) ([]string, error) {
	var metaList []string
	cacheKey := utilcache.BuildAgentKey(metadataMapperCachePrefix, nodeName)

	metaBundleInterface, found := utilcache.Cache.Get(cacheKey)
	if !found {
		log.Tracef("no metadata was found for the pod %s on node %s", podName, nodeName)
		return nil, nil
	}

	metaBundle, ok := metaBundleInterface.(*MetadataMapperBundle)
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
	log.Debugf("CacheKey: %s, with %d services", cacheKey, len(serviceList))
	for _, s := range serviceList {
		metaList = append(metaList, fmt.Sprintf("kube_service:%s", s))
	}

	return metaList, nil
}
