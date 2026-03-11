// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workloadconfig

import (
	"fmt"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxRetries = 3
	// reconcileKey is a fixed key used for the workqueue since we always do a full sync.
	reconcileKey = "reconcile"
)

var gvrDWC = datadoghq.GroupVersion.WithResource("datadogworkloadconfigs")

// WorkloadConfigCRDController watches DatadogWorkloadConfig CRDs and delegates reconciliation
// to registered ConfigSectionHandler implementations.
type WorkloadConfigCRDController struct {
	lister                cache.GenericLister
	synced                cache.InformerSynced
	workqueue             workqueue.TypedRateLimitingInterface[string]
	isLeader              func() bool
	leadershipChangeNotif <-chan struct{}
	handlers              []ConfigSectionHandler
}

// NewWorkloadConfigCRDController creates a new WorkloadConfigCRDController.
func NewWorkloadConfigCRDController(
	informerFactory dynamicinformer.DynamicSharedInformerFactory,
	isLeader func() bool,
	leadershipChangeNotif <-chan struct{},
	handlers []ConfigSectionHandler,
) (*WorkloadConfigCRDController, error) {
	dwcInformer := informerFactory.ForResource(gvrDWC)

	c := &WorkloadConfigCRDController{
		lister: dwcInformer.Lister(),
		synced: dwcInformer.Informer().HasSynced,
		workqueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedItemBasedRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "workloadconfig"},
		),
		isLeader:              isLeader,
		leadershipChangeNotif: leadershipChangeNotif,
		handlers:              handlers,
	}

	if _, err := dwcInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { c.enqueue() },
		UpdateFunc: func(_, _ interface{}) { c.enqueue() },
		DeleteFunc: func(_ interface{}) { c.enqueue() },
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to workloadconfig informer: %w", err)
	}

	log.Info("Instrumentation Controller started")
	return c, nil
}

// Run starts the controller workers and blocks until stopCh is closed.
func (c *WorkloadConfigCRDController) Run(stopCh <-chan struct{}) {
	defer c.workqueue.ShutDown()

	log.Info("Starting Instrumentation controller (waiting for cache sync)")
	if !cache.WaitForCacheSync(stopCh, c.synced) {
		log.Error("Failed to wait for Instrumentation caches to sync")
		return
	}
	log.Info("Instrumentation controller cache synced, starting worker")

	go c.worker()
	go c.watchLeadershipChanges(stopCh)

	<-stopCh
	log.Info("Stopping Instrumentation controller")
}

func (c *WorkloadConfigCRDController) enqueue() {
	c.workqueue.AddRateLimited(reconcileKey)
}

// watchLeadershipChanges watches for leadership changes and enqueues a reconcile
// when this instance becomes leader, ensuring CRs that existed before leadership
// acquisition are processed promptly.
func (c *WorkloadConfigCRDController) watchLeadershipChanges(stopCh <-chan struct{}) {
	for {
		select {
		case <-c.leadershipChangeNotif:
			if c.isLeader() {
				log.Warn("Instrumentation controller gained leadership, enqueuing reconciliation")
				c.enqueue()
			}
		case <-stopCh:
			return
		}
	}
}

func (c *WorkloadConfigCRDController) worker() {
	for c.processNext() {
	}
}

func (c *WorkloadConfigCRDController) processNext() bool {
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
		log.Warnf("Error reconciling Instrumentation (will retry): %v", err)
	} else {
		log.Errorf("Error reconciling Instrumentation after %d retries: %v", maxRetries, err)
		c.workqueue.Forget(key)
	}
	return true
}

// reconcile lists all CRs, converts them, and fans out to each handler.
func (c *WorkloadConfigCRDController) reconcile() error {
	if !c.isLeader() {
		return nil
	}

	objects, err := c.lister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list DatadogWorkloadConfigs: %w", err)
	}

	crs := make([]*datadoghq.DatadogWorkloadConfig, 0, len(objects))
	for _, obj := range objects {
		dwc, err := unstructuredToWorkloadConfig(obj)
		if err != nil {
			log.Warnf("Skipping malformed DatadogWorkloadConfig: %v", err)
			continue
		}
		crs = append(crs, dwc)
	}

	var firstErr error
	for _, h := range c.handlers {
		if err := h.Reconcile(crs); err != nil {
			log.Errorf("Handler %q failed: %v", h.Name(), err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// unstructuredToWorkloadConfig converts an unstructured object to a DatadogWorkloadConfig.
func unstructuredToWorkloadConfig(obj interface{}) (*datadoghq.DatadogWorkloadConfig, error) {
	unstrObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("could not cast to Unstructured: %T", obj)
	}
	dwc := &datadoghq.DatadogWorkloadConfig{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstrObj.UnstructuredContent(), dwc); err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to DatadogWorkloadConfig: %w", err)
	}
	return dwc, nil
}
