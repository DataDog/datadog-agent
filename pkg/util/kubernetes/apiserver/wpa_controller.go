// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"time"

	apis_v1alpha1 "github.com/DataDog/watermarkpodautoscaler/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/watermarkpodautoscaler/pkg/client/informers/externalversions/datadoghq/v1alpha1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RunWPA starts the controller to process events about Watermark Pod Autoscalers
func (h *AutoscalersController) RunWPA(stopCh <-chan struct{}) {
	defer h.WPAqueue.ShutDown()

	log.Infof("Starting WPA Controller ... ")
	defer log.Infof("Stopping WPA Controller")
	if !cache.WaitForCacheSync(stopCh, h.wpaListerSynced) {
		return
	}
	go wait.Until(h.workerWPA, time.Second, stopCh)
	<-stopCh
}

// ExtendToWPAController adds the handlers to the AutoscalersController to support WPAs
func ExtendToWPAController(h *AutoscalersController, wpaInformer v1alpha1.WatermarkPodAutoscalerInformer) {
	wpaInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    h.addWPAutoscaler,
			UpdateFunc: h.updateWPAutoscaler,
			DeleteFunc: h.deleteWPAutoscaler,
		},
	)
	h.WPAqueue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter(), "wpa-autoscalers")
	h.wpaLister = wpaInformer.Lister()
	h.wpaListerSynced = wpaInformer.Informer().HasSynced
}

func (h *AutoscalersController) workerWPA() {
	for h.processNextWPA() {
	}
}

func (h *AutoscalersController) processNextWPA() bool {
	key, quit := h.WPAqueue.Get()
	if quit {
		log.Error("WPA controller HPAqueue is shutting down, stopping processing")
		return false
	}
	log.Tracef("Processing %s", key)
	defer h.WPAqueue.Done(key)

	err := h.syncWPA(key)
	h.handleErr(err, key)

	// Debug output for unit tests only
	if h.autoscalers != nil {
		h.autoscalers <- key
	}
	return true
}

func (h *AutoscalersController) syncWPA(key interface{}) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	ns, name, err := cache.SplitMetaNamespaceKey(key.(string))
	if err != nil {
		log.Errorf("Could not split the key: %v", err)
		return err
	}

	wpaCached, err := h.wpaLister.WatermarkPodAutoscalers(ns).Get(name)
	switch {
	case errors.IsNotFound(err):
		log.Infof("WatermarkPodAutoscaler %v has been deleted but was not caught in the EventHandler. GC will cleanup.", key)
	case err != nil:
		log.Errorf("Unable to retrieve Watermark Pod Autoscaler %v from store: %v", key, err)
	default:
		if wpaCached == nil {
			log.Errorf("Could not parse empty wpa %s/%s from local store", ns, name)
			return ErrIsEmpty
		}
		emList := autoscalers.InspectWPA(wpaCached)
		if len(emList) == 0 {
			return nil
		}
		newMetrics := h.hpaProc.ProcessEMList(emList)
		// The syncWPA can also interact with the overflowing store.
		h.mu.Lock()
		if len(newMetrics)+h.metricsProcessedCount > maxMetricsCount {
			log.Warnf("Currently processing %d metrics, skipping %s/%s as we can't process more than %d metrics",
				h.metricsProcessedCount, wpaCached.Namespace, wpaCached.Name, maxMetricsCount)
			h.overFlowingAutoscalers[wpaCached.UID] = len(newMetrics)
			return nil
		}
		if _, ok := h.overFlowingAutoscalers[wpaCached.UID]; ok {
			log.Debugf("Previously ignored HPA %s/%s will now be processed", wpaCached.Namespace, wpaCached.Name)
			delete(h.overFlowingAutoscalers, wpaCached.UID)
		}
		h.mu.Unlock()
		h.toStore.m.Lock()
		for metric, value := range newMetrics {
			// We should only insert placeholders in the local cache.
			h.toStore.data[metric] = value
		}
		h.toStore.m.Unlock()

		log.Tracef("Local batch cache of WPA is %v", h.toStore.data)
	}

	return err
}

func (h *AutoscalersController) addWPAutoscaler(obj interface{}) {
	newAutoscaler, ok := obj.(*apis_v1alpha1.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Expected an WatermarkPodAutoscaler type, got: %v", obj)
		return
	}
	log.Debugf("Adding WPA %s/%s", newAutoscaler.Namespace, newAutoscaler.Name)
	h.enqueueWPA(newAutoscaler)
}

func (h *AutoscalersController) updateWPAutoscaler(old, obj interface{}) {
	newAutoscaler, ok := obj.(*apis_v1alpha1.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Expected an WatermarkPodAutoscaler type, got: %v", obj)
		return
	}
	oldAutoscaler, ok := old.(*apis_v1alpha1.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Expected an WatermarkPodAutoscaler type, got: %v", old)
		h.enqueueWPA(newAutoscaler) // We still want to enqueue the newAutoscaler to get the new change
		return
	}

	if !autoscalers.WPAutoscalerMetricsUpdate(newAutoscaler, oldAutoscaler) {
		log.Tracef("Update received for the %s/%s, without a relevant change to the configuration", newAutoscaler.Namespace, newAutoscaler.Name)
		return
	}
	// Need to delete the old object from the local cache. If the labels have changed, the syncAutoscaler would not override the old key.
	toDelete := autoscalers.InspectWPA(oldAutoscaler)
	h.deleteFromLocalStore(toDelete)
	// We re-evaluate if the WPA can be processed in syncWPA, subsequently to the enqueue.
	h.mu.Lock()
	if _, ok := h.overFlowingAutoscalers[oldAutoscaler.UID]; !ok {
		h.metricsProcessedCount -= len(toDelete)
	}
	delete(h.overFlowingAutoscalers, oldAutoscaler.UID)
	h.mu.Unlock()
	log.Tracef("Processing update event for wpa %s/%s with configuration: %s", newAutoscaler.Namespace, newAutoscaler.Name, newAutoscaler.Annotations)
	h.enqueueWPA(newAutoscaler)
}

// Processing the Delete Events in the Eventhandler as obj is deleted from the local store thereafter.
// Only here can we retrieve the content of the WPA to properly process and delete it.
// FIXME we could have an update in the WPAqueue while processing the deletion, we should make
// sure we process them in order instead. For now, the gc logic allows us to recover.
func (h *AutoscalersController) deleteWPAutoscaler(obj interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	toDelete := &custommetrics.MetricsBundle{}
	deletedWPA, ok := obj.(*apis_v1alpha1.WatermarkPodAutoscaler)
	if ok {
		toDelete.External = autoscalers.InspectWPA(deletedWPA)
		h.deleteFromLocalStore(toDelete.External)
		log.Debugf("Deleting %s/%s from the local cache", deletedWPA.Namespace, deletedWPA.Name)
		if !h.le.IsLeader() {
			return
		}
		log.Infof("Deleting entries of metrics from Ref %s/%s in the Global Store", deletedWPA.Namespace, deletedWPA.Name)
		if err := h.store.DeleteExternalMetricValues(toDelete); err != nil {
			h.enqueueWPA(deletedWPA)
			return
		}
		// Only decrease the count of processed metrics if we are able to successfully remove them from the global store.
		// TODO pop WPAs from h.overFlowingAutoscalers and start processing the one(s) we have been ignoring up to the maxMetricsCount
		// Current behavior: HPA will be evaluated next resync and processed if it does not have too many metrics.
		if _, ok := h.overFlowingAutoscalers[deletedWPA.UID]; !ok {
			h.metricsProcessedCount -= len(toDelete.External)
		}
		delete(h.overFlowingAutoscalers, deletedWPA.UID)
		return
	}

	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		log.Errorf("Could not get object from tombstone %#v", obj)
		return
	}

	deletedWPA, ok = tombstone.Obj.(*apis_v1alpha1.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Tombstone contained object that is not an Autoscaler: %#v", obj)
		return
	}

	log.Debugf("Deleting Metrics from WPA %s/%s", deletedWPA.Namespace, deletedWPA.Name)
	toDelete.External = autoscalers.InspectWPA(deletedWPA)
	log.Debugf("Deleting %s/%s from the local cache", deletedWPA.Namespace, deletedWPA.Name)
	h.deleteFromLocalStore(toDelete.External)
	if err := h.store.DeleteExternalMetricValues(toDelete); err != nil {
		h.enqueueWPA(deletedWPA)
		return
	}
	// Only decrease the count of processed metrics if the HPA was not ignored and if we are able to successfully remove them from the global store.
	// TODO pop WPAs from h.overFlowingAutoscalers and start processing the one(s) we have been ignoring, up to the maxMetricsCount
	if _, ok := h.overFlowingAutoscalers[deletedWPA.UID]; !ok {
		h.metricsProcessedCount -= len(toDelete.External)
	}
	delete(h.overFlowingAutoscalers, deletedWPA.UID)
}
