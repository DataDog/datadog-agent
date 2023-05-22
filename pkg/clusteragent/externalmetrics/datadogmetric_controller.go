// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	maxRetry             int    = 3
	requeueDelaySeconds  int    = 2
	ddmControllerStoreID string = "ddmc"
)

var (
	gvrDDM  = datadoghq.GroupVersion.WithResource("datadogmetrics")
	metaDDM = metav1.TypeMeta{
		Kind:       "DatadogMetric",
		APIVersion: "datadoghq.com/v1alpha1",
	}
)

// DatadogMetricController watches DatadogMetric to build an internal view of current DatadogMetric state.
// * It allows any ClusterAgent (even non leader) to answer quickly to Autoscalers queries
// * It allows leader to know the list queries to send to DD
type DatadogMetricController struct {
	clientSet dynamic.Interface
	lister    cache.GenericLister
	synced    cache.InformerSynced
	workqueue workqueue.RateLimitingInterface
	store     *DatadogMetricsInternalStore
	isLeader  func() bool
	context   context.Context
}

// NewDatadogMetricController returns a new AutoscalersController
func NewDatadogMetricController(client dynamic.Interface, informer dynamicinformer.DynamicSharedInformerFactory, isLeader func() bool, store *DatadogMetricsInternalStore) (*DatadogMetricController, error) {
	if store == nil {
		return nil, fmt.Errorf("Store must be initialized")
	}

	datadogMetricsInformer := informer.ForResource(gvrDDM)
	c := &DatadogMetricController{
		clientSet: client,
		lister:    datadogMetricsInformer.Lister(),
		synced:    datadogMetricsInformer.Informer().HasSynced,
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter(), "datadogmetrics"),
		store:     store,
		isLeader:  isLeader,
	}

	datadogMetricsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueue,
		DeleteFunc: c.enqueue,
		UpdateFunc: func(obj, new interface{}) {
			c.enqueue(new)
		},
	})

	// We use an observer on the store to propagate events as soon as possible
	c.store.RegisterObserver(DatadogMetricInternalObserver{
		SetFunc: c.enqueueID,
	})

	return c, nil
}

// Run starts the controller to handle DatadogMetrics
func (c *DatadogMetricController) Run(ctx context.Context) {
	if ctx == nil {
		log.Errorf("Cannot run with a nil context")
		return
	}
	c.context = ctx

	defer c.workqueue.ShutDown()

	log.Infof("Starting DatadogMetric Controller (waiting for cache sync)")
	if !cache.WaitForCacheSync(ctx.Done(), c.synced) {
		log.Errorf("Failed to wait for DatadogMetric caches to sync")
		return
	}

	go wait.Until(c.worker, time.Second, ctx.Done())

	log.Infof("Started DatadogMetric Controller (cache sync finished)")
	<-ctx.Done()
	log.Infof("Stopping DatadogMetric Controller")
	return
}

func (c *DatadogMetricController) worker() {
	for c.process() {
	}
}

func (c *DatadogMetricController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

func (c *DatadogMetricController) enqueueID(id, sender string) {
	// Do not enqueue our own updates (avoid infinite loops)
	if sender != ddmControllerStoreID {
		c.workqueue.AddRateLimited(id)
	}
}

func (c *DatadogMetricController) process() bool {
	key, shutdown := c.workqueue.Get()
	if shutdown {
		log.Infof("DatadogMetric Controller: Caught stop signal in workqueue")
		return false
	}

	defer c.workqueue.Done(key)

	err := c.processDatadogMetric(key)
	if err == nil {
		c.workqueue.Forget(key)
	} else {
		numRequeues := c.workqueue.NumRequeues(key)
		if numRequeues >= maxRetry {
			c.workqueue.Forget(key)
		}
		log.Errorf("Impossible to synchronize DatadogMetric (attempt #%d): %s, err: %v", numRequeues, key, err)
	}

	return true
}

func (c *DatadogMetricController) processDatadogMetric(key interface{}) error {
	datadogMetricKey := key.(string)
	log.Debugf("Processing DatadogMetric: %s", datadogMetricKey)

	ns, name, err := cache.SplitMetaNamespaceKey(datadogMetricKey)
	if err != nil {
		return fmt.Errorf("Could not split the key: %v", err)
	}

	datadogMetricCached := &datadoghq.DatadogMetric{}
	datadogMetricCachedObj, err := c.lister.ByNamespace(ns).Get(name)
	if err == nil {
		err = UnstructuredIntoDDM(datadogMetricCachedObj, datadogMetricCached)
	}

	switch {
	case errors.IsNotFound(err):
		// We ignore not found here as we may need to create a DatadogMetric later
		datadogMetricCached = nil
	case err != nil:
		return fmt.Errorf("Unable to retrieve DatadogMetric: %w", err)
	case datadogMetricCached == nil:
		return fmt.Errorf("Could not parse empty DatadogMetric from local cache")
	}

	// No error path, check what to do with this event
	if c.isLeader() {
		err = c.syncDatadogMetric(ns, name, datadogMetricKey, datadogMetricCached)
		if err != nil {
			return err
		}
	} else {
		if datadogMetricCached != nil {
			// Feeding local cache with DatadogMetric information
			c.store.Set(datadogMetricKey, model.NewDatadogMetricInternal(datadogMetricKey, *datadogMetricCached), ddmControllerStoreID)
			setDatadogMetricTelemetry(datadogMetricCached)
		} else {
			c.store.Delete(datadogMetricKey, ddmControllerStoreID)
			unsetDatadogMetricTelemetry(ns, name)
		}
	}

	return nil
}

// Synchronize DatadogMetric state between internal store and Kubernetes objects
// Make sure any `return` has the proper store Unlock
func (c *DatadogMetricController) syncDatadogMetric(ns, name, datadogMetricKey string, datadogMetric *datadoghq.DatadogMetric) error {
	datadogMetricInternal := c.store.LockRead(datadogMetricKey, true)
	if datadogMetricInternal == nil {
		if datadogMetric != nil {
			// If we don't have an instance locally, we trust Kubernetes and store it locally
			c.store.UnlockSet(datadogMetricKey, model.NewDatadogMetricInternal(datadogMetricKey, *datadogMetric), ddmControllerStoreID)
		} else {
			// If datadogMetric == nil, both objects are nil, nothing to do
			c.store.Unlock(datadogMetricKey)
		}

		return nil
	}

	// If DatadogMetric object is not present in Kubernetes, we need to clear our store (removed by user) or create it (autogen)
	if datadogMetric == nil {
		if datadogMetricInternal.Autogen && !datadogMetricInternal.Deleted {
			err := c.createDatadogMetric(ns, name, datadogMetricInternal)
			c.store.Unlock(datadogMetricKey)
			return err
		}

		// Already deleted in Kube, cleaning internal store
		c.store.UnlockDelete(datadogMetricKey, ddmControllerStoreID)
		return nil
	}

	// Objects exists in both places (local store and K8S), we need to sync them
	// Spec source of truth is Kubernetes object
	// Status source of truth is our local store
	// Except for autogen where internal store is only source of truth
	if datadogMetricInternal.Autogen && datadogMetricInternal.Deleted {
		// We send the delete and we'll clean-up internal store when we receive deleted event
		c.store.Unlock(datadogMetricKey)
		// We add a requeue in case the deleted event is lost
		c.workqueue.AddAfter(datadogMetricKey, time.Duration(requeueDelaySeconds)*time.Second)
		return c.deleteDatadogMetric(ns, name)
	}

	datadogMetricInternal.UpdateFrom(*datadogMetric)
	defer c.store.UnlockSet(datadogMetricInternal.ID, *datadogMetricInternal, ddmControllerStoreID)

	if datadogMetricInternal.IsNewerThan(datadogMetric.Status) {
		err := c.updateDatadogMetric(ns, name, datadogMetricInternal, datadogMetric)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *DatadogMetricController) createDatadogMetric(ns, name string, datadogMetricInternal *model.DatadogMetricInternal) error {
	log.Infof("Creating DatadogMetric: %s/%s", ns, name)
	datadogMetric := &datadoghq.DatadogMetric{
		TypeMeta: metaDDM,
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: datadoghq.DatadogMetricSpec{
			Query: datadogMetricInternal.RawQuery(),
		},
		Status: *datadogMetricInternal.BuildStatus(nil),
	}

	if datadogMetricInternal.Autogen {
		if len(datadogMetricInternal.ExternalMetricName) == 0 {
			return fmt.Errorf("Unable to create autogen DatadogMetric %s/%s without ExternalMetricName", ns, name)
		}

		datadogMetric.Spec.ExternalMetricName = datadogMetricInternal.ExternalMetricName
	}

	datadogMetricObj := &unstructured.Unstructured{}
	if err := UnstructuredFromDDM(datadogMetric, datadogMetricObj); err != nil {
		return err
	}
	_, err := c.clientSet.Resource(gvrDDM).Namespace(ns).Create(context.TODO(), datadogMetricObj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("Unable to create DatadogMetric: %s/%s, err: %v", ns, name, err)
	}

	setDatadogMetricTelemetry(datadogMetric)

	return nil
}

func (c *DatadogMetricController) updateDatadogMetric(ns, name string, datadogMetricInternal *model.DatadogMetricInternal, datadogMetric *datadoghq.DatadogMetric) error {
	newStatus := datadogMetricInternal.BuildStatus(&datadogMetric.Status)
	if newStatus != nil {
		log.Debugf("Updating status of DatadogMetric: %s/%s", ns, name)
		datadogMetric := &datadoghq.DatadogMetric{
			TypeMeta: metaDDM,
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       ns,
				Name:            name,
				ResourceVersion: datadogMetric.ResourceVersion,
			},
			Status: *newStatus,
		}
		datadogMetricObj := &unstructured.Unstructured{}
		if err := UnstructuredFromDDM(datadogMetric, datadogMetricObj); err != nil {
			return err
		}
		_, err := c.clientSet.Resource(gvrDDM).Namespace(ns).UpdateStatus(context.TODO(), datadogMetricObj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("Unable to update DatadogMetric: %s/%s, err: %v", ns, name, err)
		}
		setDatadogMetricTelemetry(datadogMetric)
	} else {
		return fmt.Errorf("Impossible to build new status for DatadogMetric: %s", datadogMetricInternal.ID)
	}

	return nil
}

func (c *DatadogMetricController) deleteDatadogMetric(ns, name string) error {
	log.Infof("Deleting DatadogMetric: %s/%s", ns, name)
	err := c.clientSet.Resource(gvrDDM).Namespace(ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("Unable to delete DatadogMetric: %s/%s, err: %v", ns, name, err)
	}
	unsetDatadogMetricTelemetry(ns, name)
	return nil
}
