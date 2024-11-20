// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Controller is a generic implementation of a Kubernetes controller processing objects that can be retrieved through Dynamic Client
// User needs to implement the Processor interface to define the processing logic
type Controller struct {
	processor Processor
	synced    cache.InformerSynced
	context   context.Context

	// Fields available to child controllers
	ID        string
	Client    dynamic.Interface
	Lister    cache.GenericLister
	Workqueue workqueue.TypedRateLimitingInterface[string]
	IsLeader  func() bool
}

// NewController returns a new workload autoscaling controller
func NewController(
	controllerID string,
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
		synced:    mainInformer.Informer().HasSynced,
		Workqueue: workqueue,
		IsLeader:  isLeader,
	}

	if _, err := mainInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueue,
		DeleteFunc: c.enqueue,
		UpdateFunc: func(_, new interface{}) {
			c.enqueue(new)
		},
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to informer: %v", err)
	}

	// We use an observer on the store to propagate events as soon as possible
	observable.RegisterObserver(Observer{
		SetFunc: c.enqueueID,
	})

	return c, nil
}

// Run starts the controller to handle objects
func (c *Controller) Run(ctx context.Context) {
	if ctx == nil {
		log.Errorf("Cannot run with a nil context")
		return
	}
	c.context = ctx

	defer c.Workqueue.ShutDown()

	log.Infof("Starting controller id: %s (waiting for cache sync)", c.ID)
	if !cache.WaitForCacheSync(ctx.Done(), c.synced) {
		log.Errorf("Failed to wait for caches to sync for controller id: %s", c.ID)
		return
	}
	log.Infof("Started controller: %s (cache sync finished)", c.ID)

	if preStart, ok := c.processor.(ProcessorPreStart); ok {
		preStart.PreStart(c.context)
		log.Debugf("PreStart done for controller id: %s", c.ID)
	}

	log.Debugf("Starting workers for controller id: %s", c.ID)
	go wait.Until(c.worker, time.Second, ctx.Done())

	<-ctx.Done()
	log.Infof("Stopping controller id: %s", c.ID)
}

func (c *Controller) worker() {
	for c.process() {
	}
}

func (c *Controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	c.Workqueue.AddRateLimited(key)
}

func (c *Controller) enqueueID(id, sender string) {
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
	} else {
		c.Workqueue.Forget(key)
	}

	return true
}
