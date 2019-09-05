// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
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
	v1alpha13 "github.com/DataDog/watermarkpodautoscaler/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/watermarkpodautoscaler/pkg/client/informers/externalversions/datadoghq/v1alpha1"
	v1alpha12 "github.com/DataDog/watermarkpodautoscaler/pkg/client/listers/datadoghq/v1alpha1"
)

const (
	// maxRetries is the maximum number of times we try to process an autoscaler before it is dropped out of the queue.
	maxRetries = 10
	// maxMetricsCount is the maximum number of metrics we can query from the backend.
	maxMetricsCount = 45
)

type PollerConfig struct {
	gcPeriodSeconds int
	refreshPeriod   int
}

type metricsBatch struct {
	data map[string]custommetrics.ExternalMetricValue
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

	wpaLister       v1alpha12.WatermarkPodAutoscalerLister
	wpaListerSynced cache.InformerSynced

	// Autoscalers that need to be added to the cache.
	queue workqueue.RateLimitingInterface

	// used in unit tests to wait until hpas are synced
	autoscalers chan interface{}

	// metricsProcessedCount keeps track of the number of metrics queried per batch to avoid going over the backend limitation
	metricsProcessedCount int
	// overFlowingHPAs keeps a map of the HPA to the number of metrics in their specs that were ignored as there are already too many metrics being processed.
	overFlowingHPAs map[types.UID]int

	toStore   metricsBatch
	hpaProc   hpa.ProcessorInterface
	store     custommetrics.Store
	clientSet kubernetes.Interface
	poller    PollerConfig
	le        LeaderElectorInterface
	mu        sync.Mutex
}

// NewAutoscalersController returns a new AutoscalersController
func NewAutoscalersController(client kubernetes.Interface, le LeaderElectorInterface, dogCl hpa.DatadogClient, autoscalingInformer autoscalersinformer.HorizontalPodAutoscalerInformer, wpaInformer v1alpha1.WatermarkPodAutoscalerInformer) (*AutoscalersController, error) {
	var err error

	h := &AutoscalersController{
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter(), "autoscalers"),
	}

	h.toStore.data = make(map[string]custommetrics.ExternalMetricValue)
	h.overFlowingHPAs = make(map[types.UID]int)

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

	// Setup the client to process the Ref and metrics
	h.hpaProc, err = hpa.NewProcessor(dogCl)
	if err != nil {
		log.Errorf("Could not instantiate the Ref Processor: %v", err.Error())
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

	wpaInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    h.addWPAutoscaler,
			UpdateFunc: h.updateWPAutoscaler,
			DeleteFunc: h.deleteWPAutoscaler,
		},
	)
	h.wpaLister = wpaInformer.Lister()
	h.wpaListerSynced = wpaInformer.Informer().HasSynced

	return h, nil
}

func (h *AutoscalersController) Run(stopCh <-chan struct{}) {
	defer h.queue.ShutDown()

	log.Infof("Starting Ref & WPA Controller ... ")
	defer log.Infof("Stopping Ref & WPA Controller")

	if !cache.WaitForCacheSync(stopCh, h.autoscalersListerSynced) {
		return
	}
	if !cache.WaitForCacheSync(stopCh, h.wpaListerSynced) {
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
				// Updating the metrics against Datadog should not affect the Ref pipeline.
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

func (h *AutoscalersController) updateExternalMetrics() {
	// Grab what is available in the Global store.
	emList, err := h.store.ListAllExternalMetricValues()
	if err != nil {
		log.Errorf("Error while retrieving external metrics from the store: %s", err)
		return
	}
	// This could be avoided, in addition to other places, if we returned a map[string]custommetrics.ExternalMetricValue from ListAllExternalMetricValues
	globalCache := make(map[string]custommetrics.ExternalMetricValue)
	for _, e := range emList {
		i := custommetrics.ExternalMetricValueKeyFunc(e)
		globalCache[i] = e
	}

	// using several metrics with the same name with different labels in the same Ref is not supported.
	h.toStore.m.Lock()
	for i, j := range h.toStore.data {
		if _, ok := globalCache[i]; !ok {
			globalCache[i] = j
		} else {
			if !reflect.DeepEqual(j.Labels, globalCache[i].Labels) {
				globalCache[i] = j
			}
		}
	}
	h.toStore.m.Unlock()

	if len(globalCache) == 0 {
		log.Debugf("No External Metrics to evaluate at the moment")
		return
	}

	updated := h.hpaProc.UpdateExternalMetrics(globalCache)
	err = h.store.SetExternalMetricValues(updated)
	if err != nil {
		log.Errorf("Not able to store the updated metrics in the Global Store: %v", err)
	}
}

// gc checks if any hpas have been deleted (possibly while the Datadog Cluster Agent was
// not running) to clean the store.
func (h *AutoscalersController) gc() {
	log.Infof("Starting garbage collection process on the Autoscalers")
	h.mu.Lock()
	defer h.mu.Unlock()

	list, err := h.autoscalersLister.HorizontalPodAutoscalers(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		log.Errorf("Could not list hpas: %v", err)
	}

	listWPA, err := h.wpaLister.WatermarkPodAutoscalers(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		return
	}
	processedList := removeIgnoredHPAs(h.overFlowingHPAs, list)
	emList, err := h.store.ListAllExternalMetricValues()
	if err != nil {
		log.Errorf("Could not list external metrics from store: %v", err)
		return
	}

	deleted := hpa.DiffExternalMetrics(list, listWPA, emList)
	if err = h.store.DeleteExternalMetricValues(deleted); err != nil {
		log.Errorf("Could not delete the external metrics in the store: %v", err)
		return
	}
	h.deleteFromLocalStore(deleted)
	log.Debugf("Done GC run. Deleted %d metrics", len(deleted))
}

// removeIgnoredHPAs is used in the gc to avoid considering the ignored HPAs
func removeIgnoredHPAs(ignored map[types.UID]int, listCached []*autoscalingv2.HorizontalPodAutoscaler) (toProcess []*autoscalingv2.HorizontalPodAutoscaler) {
	for _, hpa := range listCached {
		if _, ok := ignored[hpa.UID]; !ok {
			toProcess = append(toProcess, hpa)
		}
	}
	return
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
		// The object was deleted before we processed the add/update handle. Local store does not have the Ref data anymore. The GC will clean up the Global Store.
		log.Infof("HorizontalPodAutoscaler %v has been deleted but was not caught in the EventHandler. GC will cleanup.", key)
	case err != nil:
		log.Errorf("Unable to retrieve Horizontal Pod Autoscaler %v from store: %v", key, err)
	default:
		if hpa == nil {
			log.Errorf("Could not parse empty hpa %s/%s from local store", ns, name)
			return ErrIsEmpty
		}
		new := h.hpaProc.ProcessHPAs(hpa)
		if len(new)+h.metricsProcessedCount > maxMetricsCount {
			log.Warnf("Currently processing %d metrics, skipping %s/%s as we can't process more than %d metrics",
				h.metricsProcessedCount, hpa.Namespace, hpa.Name, maxMetricsCount)
			h.overFlowingHPAs[hpa.UID] = len(new)
			return nil
		}
		if _, ok := h.overFlowingHPAs[hpa.UID]; ok {
			log.Debugf("Previously ignored HPA %s/%s will now be processed", hpa.Namespace, hpa.Name)
			delete(h.overFlowingHPAs, hpa.UID)
		}
		h.toStore.m.Lock()
		for metric, value := range new {
			// We should only insert placeholders in the local cache.
			h.toStore.data[metric] = value
		}
		h.toStore.m.Unlock()
		h.metricsProcessedCount += len(new)
		log.Tracef("Local batch cache of Ref is %v", h.toStore.data)
	}
	if err == nil {
		return nil
	}
	wpa, err := h.wpaLister.WatermarkPodAutoscalers(ns).Get(name)
	switch {
	case errors.IsNotFound(err):
		log.Infof("WatermarkPodAutoscaler %v has been deleted but was not caught in the EventHandler. GC will cleanup.", key)
	case err != nil:
		log.Errorf("Unable to retrieve Watermark Pod Autoscaler %v from store: %v", key, err)
	default:
		if wpa == nil {
			log.Errorf("Could not parse empty hpa %s/%s from local store", ns, name)
			return ErrIsEmpty
		}
		new := h.hpaProc.ProcessWPAs(wpa)
		h.toStore.m.Lock()
		for metric, value := range new {
			// We should only insert placeholders in the local cache.
			h.toStore.data[metric] = value
		}
		h.toStore.m.Unlock()

		log.Tracef("Local batch cache of WPA is %v", h.toStore.data)
	}

	return err
}

func (h *AutoscalersController) addWPAutoscaler(obj interface{}) {
	newAutoscaler, ok := obj.(*v1alpha13.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Expected an WatermarkPodAutoscaler type, got: %v", obj)
		return
	}
	log.Debugf("Adding WPA %s/%s", newAutoscaler.Namespace, newAutoscaler.Name)
	h.enqueue(newAutoscaler)
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

func (h *AutoscalersController) updateWPAutoscaler(old, obj interface{}) {
	newAutoscaler, ok := obj.(*v1alpha13.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Expected an HorizontalPodAutoscaler type, got: %v", obj)
		return
	}
	oldAutoscaler, ok := old.(*v1alpha13.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Expected an HorizontalPodAutoscaler type, got: %v", old)
		h.enqueue(newAutoscaler) // We still want to enqueue the newAutoscaler to get the new change
		return
	}

	if !hpa.WPAutoscalerMetricsUpdate(newAutoscaler, oldAutoscaler) {
		log.Tracef("Update received for the %s/%s, without a relevant change to the configuration", newAutoscaler.Namespace, newAutoscaler.Name)
		return
	}
	// Need to delete the old object from the local cache. If the labels have changed, the syncAutoscaler would not override the old key.
	toDelete := hpa.InspectWPA(oldAutoscaler)
	h.deleteFromLocalStore(toDelete)

	log.Tracef("Processing update event for autoscaler %s/%s with configuration: %s", newAutoscaler.Namespace, newAutoscaler.Name, newAutoscaler.Annotations)
	h.enqueue(newAutoscaler)
}

// the AutoscalersController does not benefit from a diffing logic.
// Adding the new obj and dropping the previous one is sufficient.
// FIXME if the metric name or scope is changed in the Ref manifest we should propagate the change
// to the Global store here
// When the maxMetricsCount is reached concurrent ADD and UPDATE events can race.
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
	// Need to delete the old object from the local cache. If the labels (tags) have changed, the syncAutoscaler would not override the old key.
	// If the oldAutoscaler is behind a HPA in the queue that maximises processedMetricsCount, it will be added to overFlowingHPAs and garbage collected.
	toDelete := hpa.Inspect(oldAutoscaler)
	h.deleteFromLocalStore(toDelete)
	// We re-evaluate if the HPA can be processed in syncAutoscaler, subsequently to the enqueue.
	h.mu.Lock()
	if _, ok := h.overFlowingHPAs[oldAutoscaler.UID]; !ok {
		h.metricsProcessedCount -= len(toDelete)
	}
	delete(h.overFlowingHPAs, oldAutoscaler.UID)
	h.mu.Unlock()
	log.Tracef("Processing update event for autoscaler %s/%s with configuration: %s", newAutoscaler.Namespace, newAutoscaler.Name, newAutoscaler.Annotations)
	h.enqueue(newAutoscaler)
}

// Processing the Delete Events in the Eventhandler as obj is deleted from the local store thereafter.
// Only here can we retrieve the content of the Ref to properly process and delete it.
// FIXME we could have an update in the queue while processing the deletion, we should make
// sure we process them in order instead. For now, the gc logic allows us to recover.
func (h *AutoscalersController) deleteAutoscaler(obj interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	deletedHPA, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if ok {
		toDelete := hpa.Inspect(deletedHPA)
		h.deleteFromLocalStore(toDelete)
		log.Debugf("Deleting %s/%s from the local cache", deletedHPA.Namespace, deletedHPA.Name)
		if !h.le.IsLeader() {
			return
		}
		log.Infof("Deleting entries of metrics from Ref %s/%s in the Global Store", deletedHPA.Namespace, deletedHPA.Name)
		if err := h.store.DeleteExternalMetricValues(toDelete); err != nil {
			h.enqueue(deletedHPA)
			return
		}
		// Only decrease the count of processed metrics if we are able to successfully remove them from the global store.
		// TODO pop HPAs from h.overFlowingHPAs and start processing the one(s) we have been ignoring up to the maxMetricsCount
		// Current behavior: HPA will be evaluated next resync and processed if it does not have too many metrics.
		if _, ok := h.overFlowingHPAs[deletedHPA.UID]; !ok {
			h.metricsProcessedCount -= len(toDelete)
		}
		delete(h.overFlowingHPAs, deletedHPA.UID)
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

	log.Debugf("Deleting Metrics from Ref %s/%s", deletedHPA.Namespace, deletedHPA.Name)
	toDelete := hpa.Inspect(deletedHPA)
	log.Debugf("Deleting %s/%s from the local cache", deletedHPA.Namespace, deletedHPA.Name)
	h.deleteFromLocalStore(toDelete)
	if err := h.store.DeleteExternalMetricValues(toDelete); err != nil {
		h.enqueue(deletedHPA)
		return
	}
	// Only decrease the count of processed metrics if the HPA was not ignored and if we are able to successfully remove them from the global store.
	// TODO pop HPAs from h.overFlowingHPAs and start processing the one(s) we have been ignoring, up to the maxMetricsCount
	if _, ok := h.overFlowingHPAs[deletedHPA.UID]; !ok {
		h.metricsProcessedCount -= len(toDelete)
	}
	delete(h.overFlowingHPAs, deletedHPA.UID)
}

// Processing the Delete Events in the Eventhandler as obj is deleted from the local store thereafter.
// Only here can we retrieve the content of the WPA to properly process and delete it.
// FIXME we could have an update in the queue while processing the deletion, we should make
// sure we process them in order instead. For now, the gc logic allows us to recover.
func (h *AutoscalersController) deleteWPAutoscaler(obj interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	deletedWPA, ok := obj.(*v1alpha13.WatermarkPodAutoscaler)
	if ok {
		toDelete := hpa.InspectWPA(deletedWPA)
		h.deleteFromLocalStore(toDelete)
		log.Debugf("Deleting %s/%s from the local cache", deletedWPA.Namespace, deletedWPA.Name)
		if !h.le.IsLeader() {
			return
		}
		log.Infof("Deleting entries of metrics from Ref %s/%s in the Global Store", deletedWPA.Namespace, deletedWPA.Name)
		if err := h.store.DeleteExternalMetricValues(toDelete); err != nil {
			h.enqueue(deletedWPA)
			return
		}
		return
	}

	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		log.Errorf("Could not get object from tombstone %#v", obj)
		return
	}

	deletedWPA, ok = tombstone.Obj.(*v1alpha13.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Tombstone contained object that is not an Autoscaler: %#v", obj)
		return
	}

	log.Debugf("Deleting Metrics from Ref %s/%s", deletedWPA.Namespace, deletedWPA.Name)
	toDelete := hpa.InspectWPA(deletedWPA)
	log.Debugf("Deleting %s/%s from the local cache", deletedWPA.Namespace, deletedWPA.Name)
	h.deleteFromLocalStore(toDelete)
	if err := h.store.DeleteExternalMetricValues(toDelete); err != nil {
		h.enqueue(deletedWPA)
		return
	}
}

func (h *AutoscalersController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	h.queue.AddRateLimited(key)
}

func (h *AutoscalersController) deleteFromLocalStore(toDelete []custommetrics.ExternalMetricValue) {
	h.toStore.m.Lock()
	for _, d := range toDelete {
		key := custommetrics.ExternalMetricValueKeyFunc(d)
		delete(h.toStore.data, key)
	}
	h.toStore.m.Unlock()
}
