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
}

type AutoscalersController struct {

	autoscalersLister       autoscalerslister.HorizontalPodAutoscalerLister
	autoscalersListerSynced cache.InformerSynced

	// Autoscalers that need to be added to the cache.
	queue workqueue.RateLimitingInterface

	// used in unit tests to wait until endpoints are synced
	autoscalers chan interface{}

	hpaToStoreGlobally  []custommetrics.ExternalMetricValue
	hpaCl	*hpa.HPAProcessor
	store custommetrics.Store
	clientSet      kubernetes.Interface
	poller		Poller
}

func NewAutoscalerController(client kubernetes.Interface, autoscalingInformer autoscalersinformer.HorizontalPodAutoscalerInformer) *AutoscalersController{
	h := &AutoscalersController{
		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "autoscalers"),
	}
	datadogCl, err := hpa.NewDatadogClient()
	if err != nil {
		// TODO log error
		return nil
	}
	h.poller = Poller{
		gcPeriodSeconds : config.Datadog.GetInt("hpa_watcher_gc_period"),
		refreshPeriod: config.Datadog.GetInt("external_metrics_provider.polling_freq"),

	}
	h.clientSet = client
	datadogHPAConfigMap := custommetrics.GetConfigmapName()
	h.store, err = custommetrics.NewConfigMapStore(client, GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		log.Errorf("Could not instantiate the local store for the External Metrics %v", err)
		return nil
	}
	h.hpaCl, err = hpa.NewHPAWatcherClient(datadogCl)

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

func (h *AutoscalersController) Run(le LeaderElectorItf, stopCh <- chan struct{}) {
	defer h.queue.ShutDown()

	log.Infof("Starting HPA Controller ... ")
	defer log.Infof("Stopping HPA Controller")

	if !cache.WaitForCacheSync(stopCh, h.autoscalersListerSynced) {
		log.Infof("wait for cache sync")
		return
	}

	h.processingLoop(le)

	go wait.Until(h.worker, time.Second, stopCh)
	<- stopCh
}

func (c *AutoscalersController) processingLoop(le LeaderElectorItf)  {
	tickerHPARefreshProcess := time.NewTicker(time.Duration(c.poller.refreshPeriod) * time.Second)
	gcPeriodSeconds := time.NewTicker(time.Duration(c.poller.gcPeriodSeconds) * time.Second)

	batchFreq := time.NewTicker(3 * time.Second) // TODO configurable

	go func() {
		for {
			select {
			case <-tickerHPARefreshProcess.C:
				if !le.IsLeader(){
					continue
				}
				// Updating the metrics against Datadog should not affect the HPA pipeline.
				// If metrics are temporarily unavailable for too long, they will become `Valid=false` and won't be evaluated.
				c.updateExternalMetrics()
			case <-gcPeriodSeconds.C:
				if !le.IsLeader(){
					continue
				}
				c.gc()
			case <-batchFreq.C:
				if !le.IsLeader(){
					continue
				}

				if len(c.hpaToStoreGlobally) == 0 {
					continue
				}
				log.Infof("Batch call, sending %#v", c.hpaToStoreGlobally)
				err := c.storeToGlobalStore(c.hpaToStoreGlobally)
				if err != nil {
					log.Errorf("Error storing the list of External Metrics to the ConfigMap: %v", err)
					continue
				}
				// Only flush the local store if the batch call was successful.
				c.hpaToStoreGlobally = nil
			}
		}
	}()
}

func (h *AutoscalersController) storeToGlobalStore(toPush  []custommetrics.ExternalMetricValue) error {
	h.store.SetExternalMetricValues(toPush)
	return nil
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

	updated := h.hpaCl.UpdateExternalMetrics(emList)
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
	emList, err := h.store.ListAllExternalMetricValues() // Legit
	if err != nil {
		log.Errorf("Could not list external metrics from store: %v", err)
		return
	}

	deleted := h.hpaCl.ComputeDeleteExternalMetrics(list, emList)

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
	log.Infof("treating key %v ", key)

	if h.autoscalers != nil {
		h.autoscalers <- key
	}
	log.Infof("end process next")
	// Creating a leader election engine to make sure only the leader writes the metrics in the configmap and queries Datadog.
	//leaderEngine, err := leaderelection.GetLeaderEngine()
	//if err != nil {
	//	log.Errorf("Could not ensure the leader election is running properly: %s", err)
	//	return
	//}

	// add the fmt.Sprintf("externalMetrics/%s", key" to the config cache.
	//cache.Delta{}
	//cache.DeltaFIFO{}
	//cache.GaugeMetric()
	//cache.ListAll()
	//cache.KeyGetter()
	//
	//leaderEngine.EnsureLeaderElectionRuns()
	log.Infof("hpatostore now %v", h.hpaToStoreGlobally)
	// Batch call to CM with h.autoscalers ?
	//if true {//leaderEngine.IsLeader() {
	//	// bulk update.
	return true
}

func (h *AutoscalersController) addAutoscaler(obj interface{}) {
	newAutoscaler, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		log.Errorf("Expected an HorizontalPodAutoscaler type, got: %v", obj)
		return
	}
	log.Infof("Adding autoscaler %s/%s", newAutoscaler.Namespace, newAutoscaler.Name)
	new := h.hpaCl.ProcessHPAs(newAutoscaler)
	log.Infof("new is %v", new)
	log.Infof("hpaToStoreGlobally is %v", h.hpaToStoreGlobally)
	h.hpaToStoreGlobally = append(h.hpaToStoreGlobally, new ...)
	log.Infof("hpaToStoreGlobally is %v", h.hpaToStoreGlobally)
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
	h.enqueue(obj)
}

func (h *AutoscalersController) deleteAutoscaler(obj interface{}) {
	_, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if ok {
		h.enqueue(obj)
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

	h.enqueue(autoscaler)
}

func (h *AutoscalersController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v", obj, err)
		return
	}
	log.Infof("enqueueing %v", key)
	h.queue.AddRateLimited(key)
	log.Infof("queue len is %#v", h.queue.Len())
}

