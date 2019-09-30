// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"sync"
	"time"

	v1alpha12 "github.com/DataDog/watermarkpodautoscaler/pkg/client/listers/datadoghq/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/util/wait"
	autoscalersinformer "k8s.io/client-go/informers/autoscaling/v2beta1"
	"k8s.io/client-go/kubernetes"
	autoscalerslister "k8s.io/client-go/listers/autoscaling/v2beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	wpaEnabled              bool
	wpaLister               v1alpha12.WatermarkPodAutoscalerLister
	wpaListerSynced         cache.InformerSynced

	// Autoscalers that need to be added to the cache.
	queue workqueue.RateLimitingInterface

	// used in unit tests to wait until hpas are synced
	autoscalers chan interface{}

	// metricsProcessedCount keeps track of the number of metrics queried per batch to avoid going over the backend limitation
	metricsProcessedCount int
	// overFlowingHPAs keeps a map of the HPA to the number of metrics in their specs that were ignored as there are already too many metrics being processed.
	overFlowingHPAs map[types.UID]int

	toStore   metricsBatch
	hpaProc   autoscalers.ProcessorInterface
	store     custommetrics.Store
	clientSet kubernetes.Interface
	poller    PollerConfig
	le        LeaderElectorInterface
	mu        sync.Mutex
}

// RunHPA starts the controller to process events about Horizontal Pod Autoscalers
func (h *AutoscalersController) RunHPA(stopCh <-chan struct{}) {
	defer h.queue.ShutDown()

	log.Infof("Starting HPA Controller ... ")
	defer log.Infof("Stopping HPA Controller")
	if !cache.WaitForCacheSync(stopCh, h.autoscalersListerSynced) {
		return
	}
	go wait.Until(h.worker, time.Second, stopCh)
	<-stopCh
}

// ExtendToHPAController adds the handlers to the AutoscalersController to support HPAs
func ExtendToHPAController(h *AutoscalersController, autoscalingInformer autoscalersinformer.HorizontalPodAutoscalerInformer) {
	autoscalingInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    h.addAutoscaler,
			UpdateFunc: h.updateAutoscaler,
			DeleteFunc: h.deleteAutoscaler,
		},
	)
	h.autoscalersLister = autoscalingInformer.Lister()
	h.autoscalersListerSynced = autoscalingInformer.Informer().HasSynced
}

func (h *AutoscalersController) worker() {
	for h.processNext() {
	}
}

func (h *AutoscalersController) processNext() bool {
	key, quit := h.queue.Get()
	if quit {
		log.Infof("HPA controller queue is shutting down, stopping processing")
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

func (h *AutoscalersController) syncAutoscalers(key interface{}) error {
	if !h.le.IsLeader() {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	ns, name, err := cache.SplitMetaNamespaceKey(key.(string))
	if err != nil {
		log.Errorf("Could not split the key: %v", err)
		return err
	}

	hpaCached, err := h.autoscalersLister.HorizontalPodAutoscalers(ns).Get(name)
	switch {
	case errors.IsNotFound(err):
		// The object was deleted before we processed the add/update handle. Local store does not have the Ref data anymore. The GC will clean up the Global Store.
		log.Infof("HorizontalPodAutoscaler %v has been deleted but was not caught in the EventHandler. GC will cleanup.", key)
	case err != nil:
		log.Errorf("Unable to retrieve Horizontal Pod Autoscaler %v from store: %v", key, err)
	default:
		if hpaCached == nil {
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
		emList := autoscalers.InspectHPA(hpaCached)
		new := h.hpaProc.ProcessEMList(emList)
		h.toStore.m.Lock()
		for metric, value := range new {
			// We should only insert placeholders in the local cache.
			h.toStore.data[metric] = value
		}
		h.toStore.m.Unlock()
		h.metricsProcessedCount += len(new)
		log.Tracef("Local batch cache of Ref is %v", h.toStore.data)
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

	if !autoscalers.AutoscalerMetricsUpdate(newAutoscaler, oldAutoscaler) {
		log.Tracef("Update received for the %s/%s, without a relevant change to the configuration", newAutoscaler.Namespace, newAutoscaler.Name)
		return
	}
	// Need to delete the old object from the local cache. If the labels have changed, the syncAutoscaler would not override the old key.
	toDelete := autoscalers.InspectHPA(oldAutoscaler)
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
		toDelete := autoscalers.InspectHPA(deletedHPA)
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
	toDelete := autoscalers.InspectHPA(deletedHPA)
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
