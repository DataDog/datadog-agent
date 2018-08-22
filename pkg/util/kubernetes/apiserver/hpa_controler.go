// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	autoscalerslister "k8s.io/client-go/listers/autoscaling/v2beta1"
	autoscalersinformer "k8s.io/client-go/informers/autoscaling/v2beta1"
	"time"
	"k8s.io/apimachinery/pkg/api/errors"

	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hpa"
	"github.com/DataDog/datadog-agent/pkg/config"
)

type Poller struct {
	gcPeriodSeconds int
	refreshPeriod   int
	batchWindow		int
}


type AutoscalersController struct {
	autoscalersLister       autoscalerslister.HorizontalPodAutoscalerLister
	autoscalersListerSynced cache.InformerSynced
	// Autoscalers that need to be added to the cache.
	queue workqueue.RateLimitingInterface
	// used in unit tests to wait until endpoints are synced
	autoscalers chan interface{}

	hpaToStoreGlobally []custommetrics.ExternalMetricValue
	hpaProc            *hpa.HPAProcessor
	store              custommetrics.Store
	clientSet          kubernetes.Interface
	poller             Poller
	le 					LeaderElectorItf
}

func NewAutoscalerController(client kubernetes.Interface, le LeaderElectorItf, autoscalingInformer autoscalersinformer.HorizontalPodAutoscalerInformer) *AutoscalersController{
	h := &AutoscalersController{
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "autoscalers"),
	}

	datadogCl, err := hpa.NewDatadogClient()
	if err != nil {
		log.Errorf("Could not instantiate Datadog Client %v", err)
		return nil
	}
	h.hpaProc, err = hpa.NewHPAWatcherClient(datadogCl)

	h.poller = Poller{
		gcPeriodSeconds : config.Datadog.GetInt("hpa_watcher_gc_period"),
		refreshPeriod: config.Datadog.GetInt("external_metrics_provider.polling_freq"),
		batchWindow: config.Datadog.GetInt("external_metrics_provider.batch_window"),
	}

	datadogHPAConfigMap := custommetrics.GetConfigmapName()
	h.store, err = custommetrics.NewConfigMapStore(client, GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		log.Errorf("Could not instantiate the local store for the External Metrics %v", err)
		return nil
	}

	h.clientSet = client
	h.le = le
	autoscalingInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:  h.addAutoscaler,
			UpdateFunc: h.updateAutoscaler,
			DeleteFunc: h.deleteAutoscaler,
		},
	)
	h.autoscalersLister = autoscalingInformer.Lister()
	h.autoscalersListerSynced = autoscalingInformer.Informer().HasSynced

	return h
}

func (h *AutoscalersController) Run( stopCh <- chan struct{}) {
	defer h.queue.ShutDown()

	log.Infof("Starting HPA Controller ... ")
	defer log.Infof("Stopping HPA Controller")

	if !cache.WaitForCacheSync(stopCh, h.autoscalersListerSynced) {
		return
	}

	h.processingLoop()

	go wait.Until(h.worker, time.Second, stopCh)
	<- stopCh
}

// processingLoop is a second go routing that schedules the garbage collection and the refreshing of external metrics
// in the GlobalStore.
func (c *AutoscalersController) processingLoop()  {
	tickerHPARefreshProcess := time.NewTicker(time.Duration(c.poller.refreshPeriod) * time.Second)
	gcPeriodSeconds := time.NewTicker(time.Duration(c.poller.gcPeriodSeconds) * time.Second)
	batchFreq := time.NewTicker(time.Duration(c.poller.batchWindow) * time.Second)

	go func() {
		for {
			select {
			case <-tickerHPARefreshProcess.C:
				if !c.le.IsLeader(){
					c.hpaToStoreGlobally = nil
					continue
				}
				// Updating the metrics against Datadog should not affect the HPA pipeline.
				// If metrics are temporarily unavailable for too long, they will become `Valid=false` and won't be evaluated.
				c.updateExternalMetrics()
			case <-gcPeriodSeconds.C:
				if !c.le.IsLeader(){
					continue
				}
				c.gc()
			case <-batchFreq.C:
				if !c.le.IsLeader() || len(c.hpaToStoreGlobally) == 0{
					continue
				}

				log.Tracef("Batch call pushing %d metrics", len(c.hpaToStoreGlobally))
				err := c.pushToGlobalStore(c.hpaToStoreGlobally)
				if err != nil {
					log.Errorf("Error storing the list of External Metrics to the ConfigMap: %v", err)
					continue
				}
			}
		}
	}()
}

func (h *AutoscalersController) pushToGlobalStore(toPush  []custommetrics.ExternalMetricValue) error {
	// reset the batch before submitting to avoid a discrepancy between the global store and the local one
	h.hpaToStoreGlobally = nil
	return h.store.SetExternalMetricValues(toPush)
}

func (h *AutoscalersController) updateExternalMetrics() {
	emList, err := h.store.ListAllExternalMetricValues()
	if err != nil {
		log.Infof("Error while retrieving external metrics from the store: %s", err)
		return
	}

	if len(emList) == 0 {
		log.Debugf("No External Metrics to evaluate at the moment")
		return
	}

	updated := h.hpaProc.UpdateExternalMetrics(emList)
	if err = h.store.SetExternalMetricValues(updated); err != nil {
		log.Errorf("Could not update the external metrics in the store: %s", err.Error())
	}
}

// gc checks if any hpas have been deleted (possibly while the Datadog Cluster Agent was
// not running) to clean the store.
func (h *AutoscalersController) gc() {
	log.Infof("Starting gc run")

	list, err := h.clientSet.AutoscalingV2beta1().HorizontalPodAutoscalers(metav1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		log.Errorf("Could not list hpas: %v", err)
		return
	}
	emList, err := h.store.ListAllExternalMetricValues()
	if err != nil {
		log.Errorf("Could not list external metrics from store: %v", err)
		return
	}

	deleted := h.hpaProc.ComputeDeleteExternalMetrics(list, emList)

	if h.store.DeleteExternalMetricValues(deleted); err != nil {
		log.Errorf("Could not delete the external metrics in the store: %v")
		return
	}

	log.Debugf("Done gc run. Deleted %d metrics", len(deleted))
}

func (h *AutoscalersController) worker() {
	for h.processNext() {
	}
}

func (h *AutoscalersController) processNext() bool {
	key, quit := h.queue.Get()
	if quit {
		return false
	}

	defer h.queue.Done(key)

	if h.autoscalers != nil {
		h.autoscalers <- key
	}

	ns, name, _ := cache.SplitMetaNamespaceKey(key.(string))
	hpa, err := h.autoscalersLister.HorizontalPodAutoscalers(ns).Get(name)

	switch {
	case errors.IsNotFound(err):
		// We missed the Deletion Event. Local store does not have the HPA data anymore. The GC will clean up the Global Store.
		log.Errorf("HorizontalPodAutoscaler %v has been deleted but was not caught in the EventHandler. GC will cleanup.", key)
	case err != nil:
		log.Errorf("Unable to retrieve Horizontal Pod Autoscaler %v from store: %v", key, err)
	default:
		if hpa == nil {
			log.Errorf("Could not parse empty hpa %s/%s from local store", ns, name)
			return true
		}
		new := h.hpaProc.ProcessHPAs(hpa)
		h.hpaToStoreGlobally = append(h.hpaToStoreGlobally, new ...)
		log.Infof("hpaToStoreGlobally is %v", h.hpaToStoreGlobally)
	}
	return true
}

func (h *AutoscalersController) addAutoscaler(obj interface{}) {
	newAutoscaler, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		log.Errorf("Expected an HorizontalPodAutoscaler type, got: %v", obj)
		return
	}
	h.enqueue(newAutoscaler)
}

// the AutoscalersController does not benefit from a diffing logic.
// Adding the new obj and dropping the previous one is sufficient.
func (h *AutoscalersController) updateAutoscaler(_, obj interface{}) {
	newAutoscaler, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		log.Errorf("Expected an HorizontalPodAutoscaler type, got: %v", obj)
		return
	}
	log.Infof("Updating autoscaler %s/%s", newAutoscaler.Namespace, newAutoscaler.Name)
	h.enqueue(newAutoscaler)
}

// Processing the Delete Events in the Eventhandler as obj is deleted from the local store thereafter.
func (h *AutoscalersController) deleteAutoscaler(obj interface{}) {
	hpa, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if ok {
		log.Debugf("Deleting Metrics from HPA %s/%s", hpa.Namespace, hpa.Name)
		log.Infof("Deleting Metrics from HPA %#v", hpa)
		toDelete := h.hpaProc.ProcessHPAs(hpa)
		h.store.DeleteExternalMetricValues(toDelete)
		h.queue.Forget(hpa)
		return
	}

	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		log.Errorf("Could not get object from tombstone %#v", obj)
		return
	}

	autoscaler, ok := tombstone.Obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		log.Errorf("Tombstone contained object that is not an Autoscaler: %#v", obj)
		return
	}

	log.Debugf("Deleting Metrics from HPA %s/%s", hpa.Namespace, hpa.Name)
	toDelete := h.hpaProc.ProcessHPAs(autoscaler)
	h.store.DeleteExternalMetricValues(toDelete)
	h.queue.Forget(hpa)
}

func (h *AutoscalersController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	log.Infof("enqueueing %v", key)
	h.queue.AddRateLimited(key)
}

