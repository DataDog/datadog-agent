// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"time"
	"reflect"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/watermarkpodautoscaler/pkg/apis/datadoghq/v1alpha1"
)

// NewAutoscalersController returns a new AutoscalersController
func NewAutoscalersController(client kubernetes.Interface, le LeaderElectorInterface, dogCl autoscalers.DatadogClient) (*AutoscalersController, error) {
	var err error

	h := &AutoscalersController{
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter(), "autoscalers"),
	}

	h.toStore.data = make(map[string]custommetrics.ExternalMetricValue)

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
	h.hpaProc, err = autoscalers.NewProcessor(dogCl)
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
	return h, nil
}

func (h *AutoscalersController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	h.queue.AddRateLimited(key)
}

// RunControllerLoop is the public method to trigger the lifecycle loop of the External Metrics store
func (h *AutoscalersController) RunControllerLoop(stopCh <-chan struct{}) {
	h.processingLoop()
}

// gc checks if any hpas or wpas have been deleted (possibly while the Datadog Cluster Agent was
// not running) to clean the store.
func (h *AutoscalersController) gc() {
	log.Infof("Starting garbage collection process on the Autoscalers")
	h.mu.Lock()
	defer h.mu.Unlock()

	list, err := h.autoscalersLister.HorizontalPodAutoscalers(metav1.NamespaceAll).List(labels.Everything())
	if err != nil {
		log.Errorf("Could not list hpas: %v", err)
	}

	listWPA := []*v1alpha1.WatermarkPodAutoscaler{}
	if h.wpaEnabled {
		listWPA, err = h.wpaLister.WatermarkPodAutoscalers(metav1.NamespaceAll).List(labels.Everything())
		if err != nil {
			log.Errorf("Error listing the WatermarkPodAutoscalers %v", err)
			return
		}
	}
	processedList := removeIgnoredHPAs(h.overFlowingHPAs, list)
	emList, err := h.store.ListAllExternalMetricValues()
	if err != nil {
		log.Errorf("Could not list external metrics from store: %v", err)
		return
	}

	deleted := hpa.DiffExternalMetrics(processedList, listWPA, emList)
	if err = h.store.DeleteExternalMetricValues(deleted); err != nil {
		log.Errorf("Could not delete the external metrics in the store: %v", err)
		return
	}
	h.deleteFromLocalStore(deleted)
	log.Debugf("Done GC run. Deleted %d metrics", len(deleted))
}

func (h *AutoscalersController) deleteFromLocalStore(toDelete []custommetrics.ExternalMetricValue) {
	h.toStore.m.Lock()
	for _, d := range toDelete {
		key := custommetrics.ExternalMetricValueKeyFunc(d)
		delete(h.toStore.data, key)
	}
	h.toStore.m.Unlock()
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

// processingLoop is a go routine that schedules the garbage collection and the refreshing of external metrics
// in the GlobalStore.
func (h *AutoscalersController) processingLoop() {
	tickerAutoscalerRefreshProcess := time.NewTicker(time.Duration(h.poller.refreshPeriod) * time.Second)
	gcPeriodSeconds := time.NewTicker(time.Duration(h.poller.gcPeriodSeconds) * time.Second)
	log.Info("we are processing")
	go func() {
		for {
			select {
			case <-tickerAutoscalerRefreshProcess.C:
				if !h.le.IsLeader() {
					continue
				}
				// Updating the metrics against Datadog should not affect the Ref pipeline.
				// If metrics are temporarily unavailable for too long, they will become `Valid=false` and won't be evaluated.
				h.updateExternalMetrics()
			case <-gcPeriodSeconds.C:
				if !h.le.IsLeader() {
					continue
				}
				h.gc()
			}
		}
	}()
}
