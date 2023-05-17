// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"fmt"
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/watermarkpodautoscaler/api/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewAutoscalersController returns a new AutoscalersController
func NewAutoscalersController(client kubernetes.Interface, eventRecorder record.EventRecorder, isLeaderFunc func() bool, dogCl autoscalers.DatadogClient) (*AutoscalersController, error) {
	var err error
	h := &AutoscalersController{
		clientSet:     client,
		isLeaderFunc:  isLeaderFunc, // only trigger GC and updateExternalMetrics by the Leader.
		HPAqueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter(), "autoscalers"),
		EventRecorder: eventRecorder,
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
	h.hpaProc = autoscalers.NewProcessor(dogCl)
	datadogHPAConfigMap := custommetrics.GetConfigmapName()
	h.store, err = custommetrics.NewConfigMapStore(client, common.GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		log.Errorf("Could not instantiate the local store for the External Metrics %v", err)
		return nil, err
	}
	return h, nil
}

func (h *AutoscalersController) enqueueWPA(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	h.WPAqueue.AddRateLimited(key)
}

func (h *AutoscalersController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	h.HPAqueue.AddRateLimited(key)
}

// RunControllerLoop is the public method to trigger the lifecycle loop of the External Metrics store
func (h *AutoscalersController) RunControllerLoop(stopCh <-chan struct{}) {
	h.processingLoop(stopCh)
}

// gc checks if any hpas or wpas have been deleted (possibly while the Datadog Cluster Agent was
// not running) to clean the store.
func (h *AutoscalersController) gc() {
	wpaEnabled := h.isWPAEnabled()
	h.mu.Lock()
	defer h.mu.Unlock()
	log.Infof("Starting garbage collection process on the Autoscalers: wpa=%v", wpaEnabled)
	wpaList := []*v1alpha1.WatermarkPodAutoscaler{}
	var err error

	if wpaEnabled {
		wpaListObj, err := h.wpaLister.List(labels.Everything())
		if err != nil {
			log.Errorf("Error listing the WatermarkPodAutoscalers %v", err)
			return
		}
		log.Debugf("Garbage collection over %d WPAs", len(wpaListObj))
		for _, obj := range wpaListObj {
			tmp := &v1alpha1.WatermarkPodAutoscaler{}
			if err := UnstructuredIntoWPA(obj, tmp); err != nil {
				log.Errorf("Unable to cast object from local cache into a WPA: %v", err)
				continue
			}
			wpaList = append(wpaList, tmp)
		}
	}

	hpaListObj, err := h.autoscalersLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Could not list hpas: %v", err)
		return
	}

	hpaList := make([]metav1.Object, 0, len(hpaListObj))
	for _, obj := range hpaListObj {
		hpa, ok := obj.(metav1.Object)
		if !ok {
			log.Errorf("Unable to cast object from local cache into HPA, got: %+v", obj)
			continue
		}

		hpaList = append(hpaList, hpa)
	}

	emList, err := h.store.ListAllExternalMetricValues()
	if err != nil {
		log.Errorf("Could not list external metrics from store: %v", err)
		return
	}

	toDelete := &custommetrics.MetricsBundle{}
	toDelete.External = autoscalers.DiffExternalMetrics(hpaList, wpaList, emList.External)
	if err = h.store.DeleteExternalMetricValues(toDelete); err != nil {
		log.Errorf("Could not delete the external metrics in the store: %v", err)
		return
	}

	h.deleteFromLocalStore(toDelete.External)

	log.Infof("Done GC run. Deleted %d metrics", len(toDelete.External))
}

func (h *AutoscalersController) deleteFromLocalStore(toDelete []custommetrics.ExternalMetricValue) {
	h.toStore.m.Lock()
	defer h.toStore.m.Unlock()
	for _, d := range toDelete {
		key := custommetrics.ExternalMetricValueKeyFunc(d)
		delete(h.toStore.data, key)
	}
}

func (h *AutoscalersController) handleErr(err error, key interface{}) {
	if err == nil {
		log.Tracef("Faithfully dropping key %v", key)
		h.HPAqueue.Forget(key)
		return
	}

	if h.HPAqueue.NumRequeues(key) < maxRetries {
		log.Debugf("Error syncing the autoscaler %v, will rety for another %d times: %v", key, maxRetries-h.HPAqueue.NumRequeues(key), err)
		h.HPAqueue.AddRateLimited(key)
		return
	}
	log.Errorf("Too many errors trying to sync the autoscaler %v, dropping out of the HPAqueue: %v", key, err)
	h.HPAqueue.Forget(key)
}

func (h *AutoscalersController) updateExternalMetrics() {
	// Grab what is available in the Global store.
	emList, err := h.store.ListAllExternalMetricValues()
	if err != nil {
		log.Errorf("Error while retrieving external metrics from the store: %s", err)
		return
	}
	if len(emList.Deprecated) != 0 {
		toDelete := &custommetrics.MetricsBundle{
			Deprecated: emList.Deprecated,
		}
		h.store.DeleteExternalMetricValues(toDelete) //nolint:errcheck
		// need to return here or to recall list as external might contain wrong data.
	}

	// This could be avoided, in addition to other places, if we returned a map[string]custommetrics.ExternalMetricValue from ListAllExternalMetricValues
	globalCache := make(map[string]custommetrics.ExternalMetricValue)
	for _, e := range emList.External {
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
func (h *AutoscalersController) processingLoop(stopCh <-chan struct{}) {
	tickerAutoscalerRefreshProcess := time.NewTicker(time.Duration(h.poller.refreshPeriod) * time.Second)
	gcPeriodSeconds := time.NewTicker(time.Duration(h.poller.gcPeriodSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-stopCh:
				return
			case <-tickerAutoscalerRefreshProcess.C:
				if !h.isLeaderFunc() {
					continue
				}
				// Updating the metrics against Datadog should not affect the Ref pipeline.
				// If metrics are temporarily unavailable for too long, they will become `Valid=false` and won't be evaluated.
				h.updateExternalMetrics()
			case <-gcPeriodSeconds.C:
				if !h.isLeaderFunc() {
					continue
				}
				h.gc()
			}
		}
	}()
}
