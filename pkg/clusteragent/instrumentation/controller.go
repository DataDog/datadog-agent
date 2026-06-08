// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package instrumentation

import (
	"context"
	"fmt"
	"sync"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxRetries = 3
)

// Controller watches DatadogInstrumentation CRs and dispatches section events to product handlers.
type Controller struct {
	statusClient dynamic.Interface
	lister       cache.GenericLister
	synced       cache.InformerSynced
	workqueue    workqueue.TypedRateLimitingInterface[string]
	handlers     []Handler
	isLeader     func() bool

	lastSeenMu sync.Mutex
	lastSeen   map[string]*datadoghq.DatadogInstrumentation
}

// NewController creates a DatadogInstrumentation controller backed by a dynamic informer.
func NewController(statusClient dynamic.Interface, informer dynamicinformer.DynamicSharedInformerFactory, handlers []Handler, isLeader func() bool) (*Controller, error) {
	datadogInstrumentationInformer := informer.ForResource(DatadogInstrumentationGVR)
	c := &Controller{
		statusClient: statusClient,
		lister:       datadogInstrumentationInformer.Lister(),
		synced:       datadogInstrumentationInformer.Informer().HasSynced,
		workqueue:    workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedItemBasedRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "datadoginstrumentations"}),
		handlers:     handlers,
		isLeader:     isLeader,
		lastSeen:     make(map[string]*datadoghq.DatadogInstrumentation),
	}

	if _, err := datadogInstrumentationInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleAdd,
		UpdateFunc: c.handleUpdate,
		DeleteFunc: c.handleDelete,
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to DatadogInstrumentation informer: %w", err)
	}

	return c, nil
}

// Run starts the controller with a single worker.
func (c *Controller) Run(ctx context.Context) {
	log.Infof("Starting DatadogInstrumentation Controller (waiting for cache sync)")
	if !cache.WaitForCacheSync(ctx.Done(), c.synced) {
		log.Errorf("Failed to wait for DatadogInstrumentation caches to sync")
		return
	}

	go c.worker(ctx)

	log.Infof("Started DatadogInstrumentation Controller (cache sync finished)")
	<-ctx.Done()
	log.Infof("Stopping DatadogInstrumentation Controller")
	c.workqueue.ShutDown()
}

func (c *Controller) worker(ctx context.Context) {
	for c.process(ctx) {
	}
}

func (c *Controller) process(ctx context.Context) bool {
	key, shutdown := c.workqueue.Get()
	if shutdown {
		log.Infof("DatadogInstrumentation Controller caught stop signal in workqueue")
		return false
	}
	defer c.workqueue.Done(key)

	if err := c.reconcile(ctx, key); err == nil {
		c.workqueue.Forget(key)
	} else {
		numRequeues := c.workqueue.NumRequeues(key)
		if numRequeues >= maxRetries {
			c.workqueue.Forget(key)
			log.Errorf("Max retries reached for DatadogInstrumentation: %s, err: %v", key, err)
		} else {
			c.workqueue.AddRateLimited(key)
			log.Warnf("Couldn't reconcile DatadogInstrumentation (attempt #%d): %s, err: %v", numRequeues, key, err)
		}
	}
	return true
}

func (c *Controller) reconcile(ctx context.Context, key string) error {
	current, err := c.getCurrent(key)
	if err != nil {
		return err
	}
	old := c.getLastSeen(key)

	if old == nil && current == nil {
		return nil
	}

	statuses := make([]HandlerStatus, 0)

	for _, handler := range c.handlers {
		eventType, ok := classifySectionEvent(handler, old, current)
		if !ok {
			continue
		}

		eventCR := current
		if eventType == EventDelete {
			eventCR = old
		}
		status, err := handler.Handle(ctx, eventType, eventCR)
		if err != nil {
			return err
		}
		statuses = append(statuses, status)
	}

	c.setLastSeen(key, current)

	// Only the leader writes status conditions back to the CR. All replicas run handlers because
	// some handlers must act on events regardless of leadership. Known gap: a handler error on a
	// follower will not be reflected in the CR status if the leader's handler succeeded.
	if !c.isLeader() {
		return nil
	}
	return updateStatusConditions(ctx, c.statusClient, current, statuses)
}

func (c *Controller) getCurrent(key string) (*datadoghq.DatadogInstrumentation, error) {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, fmt.Errorf("invalid key %q: %w", key, err)
	}

	obj, err := c.lister.ByNamespace(ns).Get(name)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	cr, err := DatadogInstrumentationFromObject(obj)
	if err != nil {
		return nil, err
	}
	return cr, nil
}

func (c *Controller) getLastSeen(key string) *datadoghq.DatadogInstrumentation {
	c.lastSeenMu.Lock()
	defer c.lastSeenMu.Unlock()
	cr := c.lastSeen[key]
	if cr != nil {
		return cr.DeepCopy()
	}
	return nil
}

func (c *Controller) setLastSeen(key string, cr *datadoghq.DatadogInstrumentation) {
	c.lastSeenMu.Lock()
	defer c.lastSeenMu.Unlock()
	if cr == nil {
		delete(c.lastSeen, key)
	} else {
		c.lastSeen[key] = cr.DeepCopy()
	}
}

func (c *Controller) enqueueKey(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Warnf("Couldn't get key for DatadogInstrumentation object %v: %v", obj, err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

func (c *Controller) handleAdd(obj interface{}) {
	c.enqueueKey(obj)
}

func (c *Controller) handleUpdate(oldObj, newObj interface{}) {
	oldAcc, oldErr := apimeta.Accessor(oldObj)
	newAcc, newErr := apimeta.Accessor(newObj)
	if oldErr == nil && newErr == nil && oldAcc.GetGeneration() == newAcc.GetGeneration() {
		return
	}
	c.enqueueKey(newObj)
}

func (c *Controller) handleDelete(obj interface{}) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	c.enqueueKey(obj)
}
