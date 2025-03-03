// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"encoding/json"
	"fmt"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kubecache "k8s.io/client-go/tools/cache"
)

type podCache struct {
	mu   sync.RWMutex
	pods map[string]*types.Pod
}

func newPodCache() *podCache {
	return &podCache{
		pods: make(map[string]*types.Pod),
	}
}

func (c *podCache) addOrUpdatePod(pod *types.Pod) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pods[pod.Metadata.UID] = pod
	log.Debugf("Pod added/updated in cache: %s/%s", pod.Metadata.Namespace, pod.Metadata.Name)
}

func (c *podCache) removePod(podUID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pods, podUID)
	log.Debugf("Pod removed from cache: UID %s", podUID)
}

func (c *podCache) getPodList() *types.PodList {
	c.mu.RLock()
	defer c.mu.RUnlock()

	podList := &types.PodList{
		Items: make([]*types.Pod, 0, len(c.pods)),
	}
	for _, pod := range c.pods {
		podList.Items = append(podList.Items, pod)
	}
	return podList
}

// initPodInformer initializes an informer for pods on the current node.
func (ku *KubeUtil) initPodInformer() {
	log.Info("Initializing Pod List Watcher for API Server-based kubelet fallback")
	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", ku.nodeName).String()

	factory := informers.NewSharedInformerFactoryWithOptions(ku.apiClient.Cl, 30*time.Second, // Resync every 30s
		informers.WithNamespace(metav1.NamespaceAll), // Watch all namespaces
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = fieldSelector
		}),
	)

	podInformer := factory.Core().V1().Pods().Informer()

	// Set up event handlers
	podInformer.AddEventHandler(kubecache.ResourceEventHandlerFuncs{
		AddFunc:    ku.handlePodAdd,
		UpdateFunc: ku.handlePodUpdate,
		DeleteFunc: ku.handlePodDelete,
	})

	// Start the factory and wait for sync
	stopCh := make(chan struct{})
	defer close(stopCh)

	go factory.Start(stopCh)

	if !cache.WaitForCacheSync(stopCh, podInformer.HasSynced) {
		log.Errorf("Timed out waiting for cache sync")
		return
	}
	log.Infof("Pod informer has synced")
}

func (ku *KubeUtil) handlePodAdd(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		log.Errorf("handlePodAdd: expected *v1.Pod but got %T", obj)
		return
	}

	kubeletPod, err := ku.transformCoreV1Pod(pod)
	if err != nil {
		log.Errorf("handlePodAdd: failed to convert pod: %v", err)
		return
	}

	ku.apiPodCache.addOrUpdatePod(kubeletPod)
	log.Infof("Pod added to cache: %s/%s", kubeletPod.Metadata.Namespace, kubeletPod.Metadata.Name)
}

// Handle Pod Update Events (updates kubelet cache)
func (ku *KubeUtil) handlePodUpdate(_, newObj interface{}) {
	newPod, ok := newObj.(*v1.Pod)
	if !ok {
		log.Errorf("handlePodUpdate: expected *v1.Pod but got %T", newObj)
		return
	}

	kubeletPod, err := ku.transformCoreV1Pod(newPod)
	if err != nil {
		log.Errorf("handlePodUpdate: failed to convert pod: %v", err)
		return
	}

	ku.apiPodCache.addOrUpdatePod(kubeletPod)
	log.Infof("Pod updated in cache: %s/%s", kubeletPod.Metadata.Namespace, kubeletPod.Metadata.Name)
}

// Handle Pod Delete Events (removes from kubelet cache)
func (ku *KubeUtil) handlePodDelete(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		log.Errorf("handlePodDelete: expected *v1.Pod but got %T", obj)
		return
	}

	ku.apiPodCache.removePod(string(pod.UID))
	log.Infof("Pod removed from cache: %s/%s", pod.Namespace, pod.Name)
}

func (ku *KubeUtil) transformCoreV1Pod(obj interface{}) (*types.Pod, error) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		return nil, fmt.Errorf("expected *v1.Pod but got %T", obj)
	}

	//This conversion is inefficient. If used in the future we should just directly copy fields from the corev1.pod to a kubelet.Pod.

	// Marshal the corev1.Pod to JSON
	data, err := json.Marshal(pod)
	if err != nil {
		log.Errorf("Error marshaling pod: %v", err)
		return nil, err
	}

	// Unmarshal into kubelettypes.Pod
	var kubeletPod types.Pod
	if err := json.Unmarshal(data, &kubeletPod); err != nil {
		log.Errorf("Error unmarshaling into kubelettypes.Pod: %v", err)
		return nil, err
	}

	log.Debugf("Transformed Pod: %s/%s", pod.Namespace, pod.Name)
	return &kubeletPod, nil
}
