// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	datadoghq "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	dd_clientset "github.com/DataDog/datadog-operator/pkg/generated/clientset/versioned"
	dd_informers "github.com/DataDog/datadog-operator/pkg/generated/informers/externalversions"
	dd_listers "github.com/DataDog/datadog-operator/pkg/generated/listers/datadoghq/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// DatadogMetricController watches DatadogMetric to build an internal view of current DatadogMetric state.
// * It allows any ClusterAgent (even non leader) to answer quickly to Autoscalers queries
// * It allows leader to know the list queries to send to DD
type DatadogMetricController struct {
	clientSet dd_clientset.Interface
	lister    dd_listers.DatadogMetricLister
	synced    cache.InformerSynced
	workqueue workqueue.RateLimitingInterface
	store     *DatadogMetricsInternalStore
	le        apiserver.LeaderElectorInterface
}

// NewAutoscalersController returns a new AutoscalersController
func NewDatadogMetricController(resyncPeriod int64, client dd_clientset.Interface, informer dd_informers.SharedInformerFactory, le apiserver.LeaderElectorInterface, store *DatadogMetricsInternalStore) (*DatadogMetricController, error) {
	if store == nil {
		return nil, fmt.Errorf("Store cannot be nil")
	}

	datadogMetricsInformer := informer.Datadoghq().V1alpha1().DatadogMetrics()
	c := &DatadogMetricController{
		clientSet: client,
		lister:    datadogMetricsInformer.Lister(),
		synced:    datadogMetricsInformer.Informer().HasSynced,
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter(), "datadogmetrics"),
		store:     store,
		le:        le,
	}

	// We use resync to sync back information to DatadogMetrics
	datadogMetricsInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueue,
		DeleteFunc: c.enqueue,
		UpdateFunc: func(obj, new interface{}) {
			c.enqueue(new)
		},
	}, time.Duration(resyncPeriod)*time.Second)

	return c, nil
}

// Run starts the controller to handle DatadogMetrics
func (c *DatadogMetricController) Run(stopCh <-chan struct{}) error {
	defer c.workqueue.ShutDown()

	log.Infof("Starting DatadogMetric Controller... ")
	if !cache.WaitForCacheSync(stopCh, c.synced) {
		return fmt.Errorf("Failed to wait for DatadogMetric caches to sync")
	}

	go wait.Until(c.worker, time.Second, stopCh)

	log.Infof("Started DatadogMetric Controller")
	<-stopCh
	log.Infof("Stopping DatadogMetric Controller")
	return nil
}

func (c *DatadogMetricController) worker() {
	for c.processDatadogMetric() {
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

func (c *DatadogMetricController) processDatadogMetric() bool {
	key, shutdown := c.workqueue.Get()
	if shutdown {
		log.Infof("DatadogMetric Controller: Caught stop signal in workqueue")
		return false
	}

	defer c.workqueue.Done(key)
	// Always Forget() as we'll requeue everything or get updates from leader every Xs
	defer c.workqueue.Forget(key)

	err := c.syncDatadogMetric(key)
	if err != nil {
		log.Errorf("Impossible to synchronize DatadogMetric: %s, err: %v", key, err)
	}

	return true
}

func (c *DatadogMetricController) syncDatadogMetric(key interface{}) error {
	datadogMetricKey := key.(string)
	log.Debugf("Processing DatadogMetric: %s", datadogMetricKey)

	ns, name, err := cache.SplitMetaNamespaceKey(datadogMetricKey)
	if err != nil {
		return fmt.Errorf("Could not split the key: %v", err)
	}

	datadogMetricCached, err := c.lister.DatadogMetrics(ns).Get(name)
	switch {
	case errors.IsNotFound(err):
		// DatadogMetric has been deleted, removing locally
		log.Debugf("Removing DatadogMetric %s locally as object deleted", datadogMetricKey)
		c.store.Delete(datadogMetricKey)
	case err != nil:
		return fmt.Errorf("Unable to retrieve DatadogMetric: %v", err)
	case datadogMetricCached == nil:
		return fmt.Errorf("Could not parse empty DatadogMetric from local cache")
	}

	// No error path, check what to do with this event
	datadogMetricInternal := c.store.LockRead(datadogMetricKey, true)
	if datadogMetricInternal == nil || !c.le.IsLeader() {
		// Feeding local cache with DatadogMetric information
		c.store.UnlockSet(datadogMetricKey, model.NewDatadogMetricInternal(datadogMetricKey, *datadogMetricCached))
	} else {
		datadogMetricInternal.UpdateFrom(datadogMetricCached.Spec)
		c.store.UnlockSet(datadogMetricInternal.Id, *datadogMetricInternal)

		c.updateDatadogMetric(ns, name, datadogMetricInternal, datadogMetricCached)
	}

	return nil
}

func (c *DatadogMetricController) updateDatadogMetric(ns, name string, datadogMetricInternal *model.DatadogMetricInternal, datadogMetric *datadoghq.DatadogMetric) {
	// Update status from current leader state if we have new updates
	if datadogMetricInternal.IsNewerThan(datadogMetric.Status) {
		newStatus := datadogMetricInternal.BuildStatus(datadogMetric.Status)
		if newStatus != nil {
			log.Debugf("Update status of DatadogMetric: %s/%s", ns, name)
			_, err := c.clientSet.DatadoghqV1alpha1().DatadogMetrics(ns).UpdateStatus(&datadoghq.DatadogMetric{
				ObjectMeta: metav1.ObjectMeta{
					Name:            name,
					ResourceVersion: datadogMetric.ResourceVersion,
				},
				Status: *newStatus,
			})

			if err != nil {
				log.Errorf("Unable to update DatadogMetric: %s/%s, err: %v", ns, name, err)
			}
		} else {
			log.Debugf("Impossible to update status for DatadogMetric: %s", datadogMetricInternal.Id)
		}
	}
}
