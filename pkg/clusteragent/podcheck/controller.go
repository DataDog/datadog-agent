// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package podcheck

import (
	"context"
	"fmt"
	"maps"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxRetries = 3
	// reconcileKey is a fixed key used for the workqueue since we always do a full sync.
	reconcileKey = "reconcile"
)

var gvrDPC = datadoghq.GroupVersion.WithResource("datadogpodchecks")

// PodCheckController watches DatadogPodCheck CRDs and writes AD-compatible
// check configs to a ConfigMap that the Node Agent reads via file-based AD.
type PodCheckController struct {
	kubeClient         kubernetes.Interface
	lister             cache.GenericLister
	synced             cache.InformerSynced
	workqueue          workqueue.TypedRateLimitingInterface[string]
	isLeader           func() bool
	configMapName      string
	configMapNamespace string
}

// NewPodCheckController creates a new PodCheckController.
func NewPodCheckController(
	informerFactory dynamicinformer.DynamicSharedInformerFactory,
	kubeClient kubernetes.Interface,
	isLeader func() bool,
	configMapName string,
	configMapNamespace string,
) (*PodCheckController, error) {
	podCheckInformer := informerFactory.ForResource(gvrDPC)

	c := &PodCheckController{
		kubeClient: kubeClient,
		lister:     podCheckInformer.Lister(),
		synced:     podCheckInformer.Informer().HasSynced,
		workqueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedItemBasedRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "podcheck"},
		),
		isLeader:           isLeader,
		configMapName:      configMapName,
		configMapNamespace: configMapNamespace,
	}

	if _, err := podCheckInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { c.enqueue() },
		UpdateFunc: func(_, _ interface{}) { c.enqueue() },
		DeleteFunc: func(_ interface{}) { c.enqueue() },
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to podcheck informer: %w", err)
	}

	return c, nil
}

// Run starts the controller workers and blocks until stopCh is closed.
func (c *PodCheckController) Run(stopCh <-chan struct{}) {
	defer c.workqueue.ShutDown()

	log.Info("Starting PodCheck controller (waiting for cache sync)")
	if !cache.WaitForCacheSync(stopCh, c.synced) {
		log.Error("Failed to wait for PodCheck caches to sync")
		return
	}
	log.Info("PodCheck controller cache synced, starting worker")

	go c.worker()

	<-stopCh
	log.Info("Stopping PodCheck controller")
}

func (c *PodCheckController) enqueue() {
	c.workqueue.AddRateLimited(reconcileKey)
}

func (c *PodCheckController) worker() {
	for c.processNext() {
	}
}

func (c *PodCheckController) processNext() bool {
	key, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}
	defer c.workqueue.Done(key)

	err := c.reconcile()
	if err == nil {
		c.workqueue.Forget(key)
		return true
	}

	if c.workqueue.NumRequeues(key) < maxRetries {
		log.Warnf("Error reconciling PodCheck ConfigMap (will retry): %v", err)
	} else {
		log.Errorf("Error reconciling PodCheck ConfigMap after %d retries: %v", maxRetries, err)
		c.workqueue.Forget(key)
	}
	return true
}

// reconcile lists all DatadogPodCheck CRs and writes their configs to the ConfigMap.
func (c *PodCheckController) reconcile() error {
	if !c.isLeader() {
		return nil
	}

	objects, err := c.lister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list DatadogPodChecks: %w", err)
	}

	data := make(map[string]string, len(objects))
	for _, obj := range objects {
		dpc, err := unstructuredToPodCheck(obj)
		if err != nil {
			log.Warnf("Skipping malformed DatadogPodCheck: %v", err)
			continue
		}

		yamlBytes, err := convertToADConfig(dpc)
		if err != nil {
			log.Warnf("Skipping DatadogPodCheck %s/%s: %v", dpc.Namespace, dpc.Name, err)
			continue
		}

		data[configMapKey(dpc)] = string(yamlBytes)
	}

	return c.updateConfigMap(data)
}

// updateConfigMap updates the ConfigMap only if the data has changed.
func (c *PodCheckController) updateConfigMap(data map[string]string) error {
	cm, err := c.kubeClient.CoreV1().ConfigMaps(c.configMapNamespace).Get(
		context.TODO(), c.configMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w", c.configMapNamespace, c.configMapName, err)
	}

	if maps.Equal(cm.Data, data) {
		return nil
	}

	cm.Data = data
	_, err = c.kubeClient.CoreV1().ConfigMaps(c.configMapNamespace).Update(
		context.TODO(), cm, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap %s/%s: %w", c.configMapNamespace, c.configMapName, err)
	}

	log.Infof("Updated podcheck ConfigMap %s/%s with %d check configs", c.configMapNamespace, c.configMapName, len(data))
	return nil
}
