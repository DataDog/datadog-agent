// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package instrumentation follows a controller architecture watching for
// DatadogInstrumentation custom resources and reconciles their state.
//
// The controller uses a dynamic informer to track add, update, and delete events
// on DatadogInstrumentation CRs. Rather than handling each event individually, it
// coalesces them through a rate-limited workqueue and performs a full-sync
// reconciliation: listing all CRs and fanning the complete set out to each
// registered ConfigSectionHandler. This means handlers always receive the full
// picture of deployed CRs on every cycle, simplifying their reconciliation logic.
//
// The controller is leader-aware and only reconciles when the current instance
// holds the leader election lock.
package instrumentation

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

var gvrDDI = datadoghq.GroupVersion.WithResource("datadoginstrumentations")

// InstrumentationController watches DatadogInstrumentation CRDs and delegates reconciliation
// to registered ConfigSectionHandler implementations.
type InstrumentationController struct {
	lister                cache.GenericLister
	synced                cache.InformerSynced
	workqueue             workqueue.TypedRateLimitingInterface[string]
	isLeader              func() bool
	leadershipChangeNotif <-chan struct{}
	handlers              []ConfigSectionHandler
}

// NewInstrumentationCRDController creates a new InstrumentationController.
func NewInstrumentationCRDController(
	informerFactory dynamicinformer.DynamicSharedInformerFactory,
	isLeader func() bool,
	leadershipChangeNotif <-chan struct{},
	handlers []ConfigSectionHandler,
) (*InstrumentationController, error) {
	dwcInformer := informerFactory.ForResource(gvrDDI)

	c := &InstrumentationController{
		lister: dwcInformer.Lister(),
		synced: dwcInformer.Informer().HasSynced,
		workqueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedItemBasedRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "instrumentation"},
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
		return nil, fmt.Errorf("cannot add event handler to instrumentation informer: %w", err)
	}

	return c, nil
}

// Run starts the controller workers and blocks until stopCh is closed.
func (c *InstrumentationController) Run(stopCh <-chan struct{}) {
	defer c.workqueue.ShutDown()

	log.Info("Starting WorkloadConfig CRD controller (waiting for cache sync)")
	if !cache.WaitForCacheSync(stopCh, c.synced) {
		log.Error("Failed to wait for WorkloadConfig CRD caches to sync")
		return
	}
	log.Info("WorkloadConfig CRD controller started")

	go c.worker()
	go c.watchLeadershipChanges(stopCh)

	<-stopCh
	log.Info("Stopping WorkloadConfig CRD controller")
}

func (c *InstrumentationController) enqueue() {
	c.workqueue.AddRateLimited(reconcileKey)
}

// watchLeadershipChanges watches for leadership changes and enqueues a reconcile
// when this instance becomes leader, ensuring CRs that existed before leadership
// acquisition are processed promptly.
func (c *InstrumentationController) watchLeadershipChanges(stopCh <-chan struct{}) {
	for {
		select {
		case <-c.leadershipChangeNotif:
			if c.isLeader() {
				log.Warn("WorkloadConfig CRD controller gained leadership, enqueuing reconciliation")
				c.enqueue()
			}
		case <-stopCh:
			return
		}
	}
}

func (c *InstrumentationController) worker() {
	for c.processNext() {
	}
}

func (c *InstrumentationController) processNext() bool {
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
		log.Warnf("Error reconciling WorkloadConfig CRD (will retry): %v", err)
	} else {
		log.Errorf("Error reconciling WorkloadConfig CRD after %d retries: %v", maxRetries, err)
		c.workqueue.Forget(key)
	}
	return true
}

// reconcile lists all CRs, converts them, and fans out to each handler.
func (c *InstrumentationController) reconcile() error {
	if !c.isLeader() {
		return nil
	}

	objects, err := c.lister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list DatadogInstrumentations: %w", err)
	}

	crs := make([]*datadoghq.DatadogInstrumentation, 0, len(objects))
	for _, obj := range objects {
		dwc, err := unstructuredToWorkloadConfig(obj)
		if err != nil {
			log.Warnf("Skipping malformed DatadogInstrumentation: %v", err)
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

// unstructuredToWorkloadConfig converts an unstructured object to a DatadogInstrumentation.
func unstructuredToWorkloadConfig(obj interface{}) (*datadoghq.DatadogInstrumentation, error) {
	unstrObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("could not cast to Unstructured: %T", obj)
	}
	dwc := &datadoghq.DatadogInstrumentation{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstrObj.UnstructuredContent(), dwc); err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to DatadogInstrumentation: %w", err)
	}
	return dwc, nil
}
