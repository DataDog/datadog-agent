// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// initTracker tracks unique keys enqueued during the initial informer
// list and signals completion when all of them have been processed at least once.
//
// Lifecycle: add (during initial list) → seal (after handler.HasSynced) → processed (by workers).
type initTracker struct {
	done    atomic.Bool
	mu      sync.Mutex
	pending map[string]struct{}
}

// add records a key from the initial informer list.
func (t *initTracker) add(key string) {
	if t.done.Load() {
		return
	}
	t.mu.Lock()
	if t.pending == nil {
		t.pending = make(map[string]struct{})
	}
	t.pending[key] = struct{}{}
	t.mu.Unlock()
}

// processed marks a key as processed. Returns true the single time all initial
// items have been processed, so the caller can log once.
func (t *initTracker) processed(key string) bool {
	if t.done.Load() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, key)
	if len(t.pending) == 0 {
		t.pending = nil
		t.done.Store(true)
		return true
	}
	return false
}

// seal must be called once all add calls are done (i.e. after
// handler.HasSynced). It handles the empty-resource case where no items were
// enqueued. Returns true if this call caused completion.
func (t *initTracker) seal() bool {
	if t.done.Load() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.pending) == 0 {
		t.pending = nil
		t.done.Store(true)
		return true
	}
	return false
}

func (t *initTracker) isDone() bool {
	return t.done.Load()
}

// Controller is a generic implementation of a Kubernetes controller processing objects that can be retrieved through Dynamic Client
// User needs to implement the Processor interface to define the processing logic
type Controller struct {
	processor   Processor
	synced      cache.InformerSynced
	context     context.Context
	initTracker initTracker

	// Fields available to child controllers
	ID        SenderID
	Client    dynamic.Interface
	Lister    cache.GenericLister
	Workqueue workqueue.TypedRateLimitingInterface[string]
	IsLeader  func() bool
}

// NewController returns a new workload autoscaling controller
func NewController(
	controllerID SenderID,
	processor Processor,
	client dynamic.Interface,
	informer dynamicinformer.DynamicSharedInformerFactory,
	gvr schema.GroupVersionResource,
	isLeader func() bool,
	observable Observable,
	workqueue workqueue.TypedRateLimitingInterface[string],
) (*Controller, error) {
	mainInformer := informer.ForResource(gvr)
	c := &Controller{
		processor: processor,
		ID:        controllerID,
		Client:    client,
		Lister:    mainInformer.Lister(),
		Workqueue: workqueue,
		IsLeader:  isLeader,
	}

	handler, err := mainInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueue,
		DeleteFunc: c.enqueue,
		UpdateFunc: func(_, new any) {
			c.enqueue(new)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cannot add event handler to informer: %v", err)
	}

	// Use handler.HasSynced rather than informer.HasSynced: it guarantees all
	// initial-list events have been delivered to our callbacks, not just that the
	// cache is populated.
	c.synced = handler.HasSynced

	// We use an observer on the store to propagate events as soon as possible
	observable.RegisterObserver(Observer{
		SetFunc: c.enqueueID,
	})

	return c, nil
}

// InitialSyncDone returns true when initial informer sync is done AND all items have been processed at least once.
func (c *Controller) InitialSyncDone() bool {
	return c.initTracker.isDone()
}

// Run starts the controller to handle objects with the given number of workers.
func (c *Controller) Run(ctx context.Context, numWorkers int) {
	if ctx == nil {
		log.Errorf("Cannot run with a nil context")
		return
	}
	c.context = ctx

	if preStart, ok := c.processor.(ProcessorPreStart); ok {
		preStart.PreStart(c.context)
		log.Debugf("PreStart done for controller id: %s", c.ID)
	}

	log.Infof("Starting controller id: %s (waiting for cache sync)", c.ID)
	if !cache.WaitForCacheSync(ctx.Done(), c.synced) {
		log.Errorf("Failed to wait for caches to sync for controller id: %s", c.ID)
		return
	}
	log.Infof("Started controller: %s (cache sync finished)", c.ID)

	if c.initTracker.seal() {
		log.Debugf("All initial items processed for controller id: %s (no initial items)", c.ID)
	}

	log.Debugf("Starting %d workers for controller id: %s", numWorkers, c.ID)
	for i := range numWorkers {
		go c.worker(i)
	}

	<-ctx.Done()
	log.Infof("Stopping controller id: %s", c.ID)
	if c.IsLeader() {
		c.Workqueue.ShutDownWithDrain()
	} else {
		c.Workqueue.ShutDown()
	}
	log.Infof("Controller stopped: %s", c.ID)
}

func (c *Controller) worker(workerID int) {
	log.Debugf("Starting worker %d for controller id: %s", workerID, c.ID)
	for c.process() {
	}
}

func (c *Controller) enqueue(obj any) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	if !c.InitialSyncDone() {
		c.initTracker.add(key)
	}
	c.Workqueue.AddRateLimited(key)
}

func (c *Controller) enqueueID(id string, sender SenderID) {
	// Do not enqueue our own updates (avoid infinite loops)
	if sender != c.ID {
		log.Tracef("Enqueueing from observer update id: %s from sender: %s", id, sender)
		c.Workqueue.AddRateLimited(id)
	}
}

func (c *Controller) process() bool {
	key, shutdown := c.Workqueue.Get()
	if shutdown {
		log.Infof("Caught stop signal in workqueue for controller id: %s", c.ID)
		return false
	}

	defer c.Workqueue.Done(key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("Could not split the key, discarding item with key: %s, err: %v", key, err)
	}

	res := c.processor.Process(c.context, key, ns, name)
	if res.RequeueAfter > 0 {
		c.Workqueue.Forget(key)
		c.Workqueue.AddAfter(key, res.RequeueAfter)
	} else if res.Requeue {
		c.Workqueue.AddRateLimited(key)
	} else { // no requeue
		c.Workqueue.Forget(key)
	}

	if c.initTracker.processed(key) {
		log.Infof("All initial items processed for controller id: %s", c.ID)
	}

	return true
}
