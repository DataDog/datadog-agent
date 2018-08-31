// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

type PollerConfig struct {
	gcPeriodSeconds int
	refreshPeriod   int
	batchWindow     int
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
}

// NewAutoscalersController returns a new AutoscalersController
func NewAutoscalersController(client kubernetes.Interface, le LeaderElectorInterface, dogCl hpa.DatadogClient, autoscalingInformer autoscalersinformer.HorizontalPodAutoscalerInformer) (*AutoscalersController, error) {
	var err error
	h := &AutoscalersController{
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "autoscalers"),
	}

	gcPeriodSeconds := config.Datadog.GetInt("hpa_watcher_gc_period")
	refreshPeriod := config.Datadog.GetInt("external_metrics_provider.refresh_period")
	batchWindow := config.Datadog.GetInt("external_metrics_provider.batch_window")

	if gcPeriodSeconds <= 0 || refreshPeriod <= 0 || batchWindow <= 0 {
		return nil, fmt.Errorf("tickers must be strictly positive in the AutoscalersController"+
			" [GC: %d s, Refresh: %d s, Batchwindow: %d s]", gcPeriodSeconds, refreshPeriod, batchWindow)
	}

	h.poller = PollerConfig{
		gcPeriodSeconds: gcPeriodSeconds,
		refreshPeriod:   refreshPeriod,
		batchWindow:     batchWindow,
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

	if err := h.le.EnsureLeaderElectionRuns(); err != nil {
		log.Errorf("Leader election process failed to start: %v", err)
		return
	}

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
	batchFreq := time.NewTicker(time.Duration(c.poller.batchWindow) * time.Second)

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
			case <-batchFreq.C:
				err := c.pushToGlobalStore()
				if err != nil {
					log.Errorf("Error storing the list of External Metrics to the ConfigMap: %v", err)
				}
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

	if !h.le.IsLeader() {
		return nil
	}
	log.Tracef("Batch call pushing %d metrics", len(localStore))
	err := h.store.SetExternalMetricValues(localStore)
	return err
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

	err := h.syncAutoscalers(key)
	if err != nil {
		log.Errorf("Could not sync HPAs %s", err)
	}

	if h.autoscalers != nil {
		h.autoscalers <- key
	}

	return true
}

func (h *AutoscalersController) syncAutoscalers(key interface{}) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key.(string))
	if err != nil {
		log.Errorf("Could not split the key: %v", err)
		return err
	}

	hpa, err := h.autoscalersLister.HorizontalPodAutoscalers(ns).Get(name)
	switch {
	case errors.IsNotFound(err):
		// The object was deleted before we processed the add/update handle. Local store does not have the HPA data anymore. The GC will clean up the Global Store.
		log.Debugf("HorizontalPodAutoscaler %v has been deleted but was not caught in the EventHandler. GC will cleanup.", key)
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
// Only here can we retrieve the content of the HPA to properly process and delete it.
func (h *AutoscalersController) deleteAutoscaler(obj interface{}) {
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
