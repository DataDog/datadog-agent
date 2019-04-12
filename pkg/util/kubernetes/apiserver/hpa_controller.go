// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"sync"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	autoscalersinformer "k8s.io/client-go/informers/autoscaling/v2beta1"
	"k8s.io/client-go/kubernetes"
	autoscalerslister "k8s.io/client-go/listers/autoscaling/v2beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hpa"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// maxRetries is the maximum number of times we try to process an autoscaler before it is dropped out of the queue.
	maxRetries = 10
)

type PollerConfig struct {
	gcPeriodSeconds int
	refreshPeriod   int
}

type metricsBatch struct {
	data []custommetrics.ExternalMetricValue
	m    sync.Mutex
}

// AutoscalersController is responsible for synchronizing horizontal pod autoscalers from the Kubernetes
// apiserver to determine the metrics that need to be provided by the custom metrics server.
// This controller also queries Datadog for the values of detected external metrics.
//
// The controller takes care to garbage collect any data while processing updates/deletes
// so that the cache does not contain data for deleted hpas
// This controller is used by the Datadog Cluster Agent and supports Kubernetes 1.10+.
type AutoscalersController struct {
	autoscalersLister       autoscalerslister.HorizontalPodAutoscalerLister
	autoscalersListerSynced cache.InformerSynced
	// Autoscalers that need to be added to the cache.
	queue workqueue.RateLimitingInterface

	// used in unit tests to wait until hpas are synced
	autoscalers chan interface{}

	toStore   metricsBatch
	hpaProc   *hpa.Processor
	store     custommetrics.Store
	clientSet kubernetes.Interface
	poller    PollerConfig
	le        LeaderElectorInterface
	mu        sync.Mutex
}

// NewAutoscalersController returns a new AutoscalersController
func NewAutoscalersController(client kubernetes.Interface, le LeaderElectorInterface, dogCl hpa.DatadogClient, autoscalingInformer autoscalersinformer.HorizontalPodAutoscalerInformer) (*AutoscalersController, error) {
	var err error
	h := &AutoscalersController{
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter(), "autoscalers"),
	}

	gcPeriodSeconds := config.Datadog.GetInt("hpa_watcher_gc_period")
	refreshPeriod := config.Datadog.GetInt("external_metrics_provider.refresh_period")

	if gcPeriodSeconds <= 0 || refreshPeriod <= 0 {
		return nil, fmt.Errorf("tickers must be strictly positive in the AutoscalersController"+
			" [GC: %d s, Refresh: %d s]", gcPeriodSeconds, refreshPeriod)
	}

	h.poller = PollerConfig{
		gcPeriodSeconds: gcPeriodSeconds,
		refreshPeriod:   refreshPeriod,
	}

	// Setup the client to process the HPA and metrics
	h.hpaProc, err = hpa.NewProcessor(dogCl)
	if err != nil {
		log.Errorf("Could not instantiate the HPA Processor: %v", err.Error())
		return nil, err
	}
	h.clientSet = client
	h.le = le // only trigger GC and updateExternalMetrics by the Leader.

	datadogHPAConfigMap := custommetrics.GetConfigmapName()
	h.store, err = custommetrics.NewConfigMapStore(client, common.GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		log.Errorf("Could not instantiate the local store for the External Metrics %v", err)
		return nil, err
	}

	autoscalingInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    h.addAutoscaler,
			UpdateFunc: h.updateAutoscaler,
			DeleteFunc: h.deleteAutoscaler,
		},
	)
	h.autoscalersLister = autoscalingInformer.Lister()
	h.autoscalersListerSynced = autoscalingInformer.Informer().HasSynced
	return h, nil
}

func (h *AutoscalersController) Run(stopCh <-chan struct{}) {
	defer h.queue.ShutDown()

	log.Infof("Starting HPA Controller ... ")
	defer log.Infof("Stopping HPA Controller")

	if !cache.WaitForCacheSync(stopCh, h.autoscalersListerSynced) {
		return
	}

	h.processingLoop()

	go wait.Until(h.worker, time.Second, stopCh)
	<-stopCh
}

// processingLoop is a go routine that schedules the garbage collection and the refreshing of external metrics
// in the GlobalStore.
func (c *AutoscalersController) processingLoop() {
	tickerHPARefreshProcess := time.NewTicker(time.Duration(c.poller.refreshPeriod) * time.Second)
	gcPeriodSeconds := time.NewTicker(time.Duration(c.poller.gcPeriodSeconds) * time.Second)

	go func() {
		for {
			select {
			case <-tickerHPARefreshProcess.C:
				if !c.le.IsLeader() {
					continue
				}
				// Updating the metrics against Datadog should not affect the HPA pipeline.
				// If metrics are temporarily unavailable for too long, they will become `Valid=false` and won't be evaluated.
				c.updateExternalMetrics()
			case <-gcPeriodSeconds.C:
				if !c.le.IsLeader() {
					continue
				}
				c.gc()
			}
		}
	}()
}

func (h *AutoscalersController) pushToGlobalStore() error {
	// reset the batch before submitting to avoid a discrepancy between the global store and the local one
	h.toStore.m.Lock()
	localStore := h.toStore.data
	h.toStore.data = nil
	h.toStore.m.Unlock()
	if localStore == nil {
		log.Trace("No batched metrics to push to the Global Store")
		return nil
	}
	if !h.le.IsLeader() {
		return nil
	}
	log.Debugf("Batch call pushing %v metrics", len(localStore))
	err := h.store.SetExternalMetricValues(localStore)
	return err
}

func (h *AutoscalersController) updateExternalMetrics() {
	// We force a flush of the most up to date metrics that might remain in the batch
	err := h.pushToGlobalStore()
	if err != nil {
		log.Errorf("Error while pushing external metrics to the store: %s", err)
		return
	}

	// prevent some update to get into the store between ListAllExternalMetricValues
	// and SetExternalMetricValues otherwise it would get erased
	h.toStore.m.Lock()
	defer h.toStore.m.Unlock()

	emList, err := h.store.ListAllExternalMetricValues()
	if err != nil {
		log.Errorf("Error while retrieving external metrics from the store: %s", err)
		return
	}

	if len(emList) == 0 {
		log.Debugf("No External Metrics to evaluate at the moment")
		return
	}

	updated := h.hpaProc.UpdateExternalMetrics(emList)
	err = h.store.SetExternalMetricValues(updated)
	if err != nil {
		log.Errorf("Not able to store the updated metrics in the Global Store: %v", err)
	}
}

// gc checks if any hpas have been deleted (possibly while the Datadog Cluster Agent was
// not running) to clean the store.
func (h *AutoscalersController) gc() {
	log.Infof("Starting gc run")
	h.mu.Lock()
	defer h.mu.Unlock()

	list, err := h.autoscalersLister.HorizontalPodAutoscalers(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		log.Errorf("Could not list hpas: %v", err)
		return
	}

	emList, err := h.store.ListAllExternalMetricValues()
	if err != nil {
		log.Errorf("Could not list external metrics from store: %v", err)
		return
	}

	deleted := hpa.DiffExternalMetrics(list, emList)
	if h.store.DeleteExternalMetricValues(deleted); err != nil {
		log.Errorf("Could not delete the external metrics in the store: %v", err)
		return
	}
	log.Debugf("Done GC run. Deleted %d metrics", len(deleted))
}

func (h *AutoscalersController) worker() {
	for h.processNext() {
	}
}

func (h *AutoscalersController) processNext() bool {
	key, quit := h.queue.Get()
	if quit {
		log.Infof("Autoscaler queue is shutting down, stopping processing")
		return false
	}
	log.Tracef("Processing %s", key)
	defer h.queue.Done(key)

	err := h.syncAutoscalers(key)
	h.handleErr(err, key)

	// Debug output for unit tests only
	if h.autoscalers != nil {
		h.autoscalers <- key
	}
	return true
}

func (h *AutoscalersController) handleErr(err error, key interface{}) {
	if err == nil {
		log.Tracef("Faithfully dropping key %v", key)
		h.queue.Forget(key)
		return
	}

	if h.queue.NumRequeues(key) < maxRetries {
		log.Debugf("Error syncing the autoscaler %v, will rety for another %d times: %v", key, maxRetries-h.queue.NumRequeues(key), err)
		h.queue.AddRateLimited(key)
		return
	}
	log.Errorf("Too many errors trying to sync the autoscaler %v, dropping out of the queue: %v", key, err)
	h.queue.Forget(key)
}

func (h *AutoscalersController) syncAutoscalers(key interface{}) error {
	if !h.le.IsLeader() {
		log.Trace("Only the leader needs to sync the Autoscalers")
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	ns, name, err := cache.SplitMetaNamespaceKey(key.(string))
	if err != nil {
		log.Errorf("Could not split the key: %v", err)
		return err
	}

	hpa, err := h.autoscalersLister.HorizontalPodAutoscalers(ns).Get(name)
	switch {
	case errors.IsNotFound(err):
		// The object was deleted before we processed the add/update handle. Local store does not have the HPA data anymore. The GC will clean up the Global Store.
		log.Infof("HorizontalPodAutoscaler %v has been deleted but was not caught in the EventHandler. GC will cleanup.", key)
	case err != nil:
		log.Errorf("Unable to retrieve Horizontal Pod Autoscaler %v from store: %v", key, err)
	default:
		if hpa == nil {
			log.Errorf("Could not parse empty hpa %s/%s from local store", ns, name)
			return ErrIsEmpty
		}
		new := h.hpaProc.ProcessHPAs(hpa)
		h.toStore.m.Lock()
		h.toStore.data = append(h.toStore.data, new...)
		log.Tracef("Local batch cache of HPA is %v", h.toStore.data)
		h.toStore.m.Unlock()
	}
	return err
}

func (h *AutoscalersController) addAutoscaler(obj interface{}) {
	newAutoscaler, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		log.Errorf("Expected an HorizontalPodAutoscaler type, got: %v", obj)
		return
	}
	log.Debugf("Adding autoscaler %s/%s", newAutoscaler.Namespace, newAutoscaler.Name)
	h.enqueue(newAutoscaler)
}

// the AutoscalersController does not benefit from a diffing logic.
// Adding the new obj and dropping the previous one is sufficient.
// FIXME if the metric name or scope is changed in the HPA manifest we should propagate the change
// to the Global store here
func (h *AutoscalersController) updateAutoscaler(old, obj interface{}) {
	newAutoscaler, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		log.Errorf("Expected an HorizontalPodAutoscaler type, got: %v", obj)
		return
	}
	oldAutoscaler, ok := old.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		log.Errorf("Expected an HorizontalPodAutoscaler type, got: %v", old)
		h.enqueue(newAutoscaler) // We still want to enqueue the newAutoscaler to get the new change
		return
	}

	if !hpa.AutoscalerMetricsUpdate(newAutoscaler, oldAutoscaler) {
		log.Tracef("Update received for the %s/%s, without a relevant change to the configuration", newAutoscaler.Namespace, newAutoscaler.Name)
		return
	}
	log.Debugf("Updating autoscaler %s/%s with configuration: %s", newAutoscaler.Namespace, newAutoscaler.Name, newAutoscaler.Annotations)
	h.enqueue(newAutoscaler)
}

// Processing the Delete Events in the Eventhandler as obj is deleted from the local store thereafter.
// Only here can we retrieve the content of the HPA to properly process and delete it.
// FIXME we could have an update in the queue while processing the deletion, we should make
// sure we process them in order instead. For now, the gc logic allows us to recover.
func (h *AutoscalersController) deleteAutoscaler(obj interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	deletedHPA, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if ok {
		log.Debugf("Deleting Metrics from HPA %s/%s", deletedHPA.Namespace, deletedHPA.Name)
		toDelete := hpa.Inspect(deletedHPA)
		h.store.DeleteExternalMetricValues(toDelete)
		h.queue.Done(deletedHPA)
		return
	}

	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		log.Errorf("Could not get object from tombstone %#v", obj)
		return
	}

	deletedHPA, ok = tombstone.Obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		log.Errorf("Tombstone contained object that is not an Autoscaler: %#v", obj)
		return
	}

	log.Debugf("Deleting Metrics from HPA %s/%s", deletedHPA.Namespace, deletedHPA.Name)
	toDelete := hpa.Inspect(deletedHPA)
	h.store.DeleteExternalMetricValues(toDelete)
	h.queue.Done(deletedHPA)
}

func (h *AutoscalersController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	h.queue.AddRateLimited(key)
}
