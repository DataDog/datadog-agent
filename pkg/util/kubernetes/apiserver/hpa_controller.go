// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// maxRetries is the maximum number of times we try to process an autoscaler before it is dropped out of the HPAqueue.
	maxRetries = 10
)

// PollerConfig holds the configuration of the metrics poller
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
	autoscalersLister       cache.GenericLister
	autoscalersListerSynced cache.InformerSynced
	wpaEnabled              bool
	wpaLister               cache.GenericLister
	wpaListerSynced         cache.InformerSynced

	// Autoscalers that need to be added to the cache.
	HPAqueue workqueue.RateLimitingInterface
	WPAqueue workqueue.RateLimitingInterface

	EventRecorder record.EventRecorder

	// used in unit tests to wait until hpas are synced
	autoscalers chan interface{}

	toStore      metricsBatch
	hpaProc      autoscalers.ProcessorInterface
	store        custommetrics.Store
	clientSet    kubernetes.Interface
	poller       PollerConfig
	isLeaderFunc func() bool
	mu           sync.Mutex
}

// RunHPA starts the controller to process events about Horizontal Pod Autoscalers
func (h *AutoscalersController) RunHPA(stopCh <-chan struct{}) {
	defer h.HPAqueue.ShutDown()

	log.Infof("Starting HPA Controller ... ")
	defer log.Infof("Stopping HPA Controller")

	if !cache.WaitForCacheSync(stopCh, h.autoscalersListerSynced) {
		return
	}

	wait.Until(h.worker, time.Second, stopCh)
}

// enableHPA adds the handlers to the AutoscalersController to support HPAs
func (h *AutoscalersController) enableHPA(client kubernetes.Interface, informerFactory informers.SharedInformerFactory) {
	hpaGVR, err := autoscalers.DiscoverHPAGroupVersionResource(client)
	if err != nil {
		log.Errorf("unable to discover HPA GroupVersionResource: %s", err)
		return
	}

	genericInformerFactory, err := informerFactory.ForResource(hpaGVR)
	if err != nil {
		log.Errorf("error creating generic informer: %s", err)
		return
	}

	informer := genericInformerFactory.Informer()
	informer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    h.addAutoscaler,
			UpdateFunc: h.updateAutoscaler,
			DeleteFunc: h.deleteAutoscaler,
		},
	)

	h.autoscalersLister = genericInformerFactory.Lister()
	h.autoscalersListerSynced = informer.HasSynced
}

func (h *AutoscalersController) worker() {
	for h.processNextHPA() {
	}
}

func (h *AutoscalersController) processNextHPA() bool {
	key, quit := h.HPAqueue.Get()
	if quit {
		log.Infof("HPA controller HPAqueue is shutting down, stopping processing")
		return false
	}
	log.Tracef("Processing %s", key)
	defer h.HPAqueue.Done(key)

	err := h.syncHPA(key)
	h.handleErr(err, key)

	// Debug output for unit tests only
	if h.autoscalers != nil {
		h.autoscalers <- key
	}
	return true
}

func (h *AutoscalersController) syncHPA(key interface{}) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	ns, name, err := cache.SplitMetaNamespaceKey(key.(string))
	if err != nil {
		log.Errorf("Could not split the key: %v", err)
		return err
	}

	hpaCached, err := h.autoscalersLister.ByNamespace(ns).Get(name)
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
		emList := autoscalers.InspectHPA(hpaCached)
		if len(emList) == 0 {
			return nil
		}
		newMetrics := h.hpaProc.ProcessEMList(emList)
		h.toStore.m.Lock()
		for metric, value := range newMetrics {
			// We should only insert placeholders in the local cache.
			h.toStore.data[metric] = value
		}
		h.toStore.m.Unlock()
		log.Tracef("Local batch cache of Ref is %v", h.toStore.data)
	}
	return err
}

func (h *AutoscalersController) addAutoscaler(obj interface{}) {
	newAutoscaler, ok := obj.(metav1.Object)
	if !ok {
		log.Errorf("Expected a metav1.Object type, got: %v", obj)
		return
	}
	log.Debugf("Adding autoscaler %s/%s", newAutoscaler.GetNamespace(), newAutoscaler.GetName())
	h.EventRecorder.Event(obj.(runtime.Object), corev1.EventTypeNormal, autoscalerNowHandleMsgEvent, "")
	h.enqueue(newAutoscaler)
}

// the AutoscalersController does not benefit from a diffing logic.
// Adding the new obj and dropping the previous one is sufficient.
// FIXME if the metric name or scope is changed in the Ref manifest we should propagate the change
// to the Global store here
func (h *AutoscalersController) updateAutoscaler(old, obj interface{}) {
	newAutoscaler, ok := obj.(metav1.Object)
	if !ok {
		log.Errorf("Expected a metav1.Object type, got: %v", obj)
		return
	}

	oldAutoscaler, ok := old.(metav1.Object)
	if !ok {
		log.Errorf("Expected a metav1.Object type, got: %v", old)
		h.enqueue(newAutoscaler) // We still want to enqueue the newAutoscaler to get the new change
		return
	}

	if !autoscalers.AutoscalerMetricsUpdate(newAutoscaler, oldAutoscaler) {
		log.Tracef("Update received for the %s/%s, without a relevant change to the configuration", newAutoscaler.GetNamespace(), newAutoscaler.GetName())
		return
	}

	// Need to delete the old object from the local cache. If the labels have changed, the syncAutoscaler would not override the old key.
	toDelete := autoscalers.InspectHPA(oldAutoscaler)
	h.deleteFromLocalStore(toDelete)
	log.Tracef("Processing update event for autoscaler %s/%s with configuration: %s", newAutoscaler.GetNamespace(), newAutoscaler.GetName(), newAutoscaler.GetAnnotations())
	h.enqueue(newAutoscaler)
}

// Processing the Delete Events in the Eventhandler as obj is deleted from the local store thereafter.
// Only here can we retrieve the content of the Ref to properly process and delete it.
// FIXME we could have an update in the HPAqueue while processing the deletion, we should make
// sure we process them in order instead. For now, the gc logic allows us to recover.
func (h *AutoscalersController) deleteAutoscaler(o interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	toDelete := &custommetrics.MetricsBundle{}

	var deletedAutoscaler metav1.Object

	switch obj := o.(type) {
	case metav1.Object:
		deletedAutoscaler = obj
	case cache.DeletedFinalStateUnknown:
		deletedAutoscaler = obj.Obj.(metav1.Object)
	default:
		log.Errorf("expected autoscaler or tombstone, got %#v", obj)
		return
	}

	toDelete.External = autoscalers.InspectHPA(deletedAutoscaler)

	h.deleteFromLocalStore(toDelete.External)
	log.Debugf("Deleting %s/%s from the local cache", deletedAutoscaler.GetNamespace(), deletedAutoscaler.GetName())

	if h.isLeaderFunc() {
		log.Infof("Deleting entries of metrics from Ref %s/%s in the Global Store", deletedAutoscaler.GetNamespace(), deletedAutoscaler.GetName())
		if err := h.store.DeleteExternalMetricValues(toDelete); err != nil {
			h.enqueue(deletedAutoscaler)
		}
	}
}
