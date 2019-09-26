package apiserver

import (
	"github.com/DataDog/watermarkpodautoscaler/pkg/client/informers/externalversions/datadoghq/v1alpha1"
	"time"
	"k8s.io/client-go/tools/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/apimachinery/pkg/util/wait"
	apis_v1alpha1 "github.com/DataDog/watermarkpodautoscaler/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-agent/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hpa"
)

func ExtendToWPAController(h *AutoscalersController, wpaInformer v1alpha1.WatermarkPodAutoscalerInformer) (*AutoscalersController, error) {
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

func (h *AutoscalersController) RunWPA(stopCh <-chan struct{}) {
	defer h.queue.ShutDown()

	log.Infof("Starting WPA Controller ... ")
	defer log.Infof("Stopping WPA Controller")
	if !cache.WaitForCacheSync(stopCh, h.wpaListerSynced) {
		return
	}
	go wait.Until(h.workerWPA, time.Second, stopCh)
	<- stopCh
}

func (h *AutoscalersController) workerWPA() {
	for h.processNextWPA() {
	}
}

func (h *AutoscalersController) processNextWPA() bool {
	key, quit := h.queue.Get()
	if quit {
		log.Infof("WPA controller queue is shutting down, stopping processing")
		return false
	}
	log.Tracef("Processing %s", key)
	defer h.queue.Done(key)

	err := h.syncWatermarkPoAutoscalers(key)
	h.handleErr(err, key)

	// Debug output for unit tests only
	if h.autoscalers != nil {
		h.autoscalers <- key
	}
	return true
}

func (h *AutoscalersController) syncWatermarkPoAutoscalers(key interface{}) error {
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

	wpa, err := h.wpaLister.WatermarkPodAutoscalers(ns).Get(name)
	switch {
	case errors.IsNotFound(err):
		log.Infof("WatermarkPodAutoscaler %v has been deleted but was not caught in the EventHandler. GC will cleanup.", key)
	case err != nil:
		log.Errorf("Unable to retrieve Watermark Pod Autoscaler %v from store: %v", key, err)
	default:
		if wpa == nil {
			log.Errorf("Could not parse empty wpa %s/%s from local store", ns, name)
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
	newAutoscaler, ok := obj.(*apis_v1alpha1.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Expected an WatermarkPodAutoscaler type, got: %v", obj)
		return
	}
	log.Debugf("Adding WPA %s/%s", newAutoscaler.Namespace, newAutoscaler.Name)
	h.enqueue(newAutoscaler)
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

	log.Tracef("Processing update event for wpa %s/%s with configuration: %s", newAutoscaler.Namespace, newAutoscaler.Name, newAutoscaler.Annotations)
	h.enqueue(newAutoscaler)
}

// Processing the Delete Events in the Eventhandler as obj is deleted from the local store thereafter.
// Only here can we retrieve the content of the WPA to properly process and delete it.
// FIXME we could have an update in the queue while processing the deletion, we should make
// sure we process them in order instead. For now, the gc logic allows us to recover.
func (h *AutoscalersController) deleteWPAutoscaler(obj interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	deletedWPA, ok := obj.(*apis_v1alpha1.WatermarkPodAutoscaler)
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

	deletedWPA, ok = tombstone.Obj.(*apis_v1alpha1.WatermarkPodAutoscaler)
	if !ok {
		log.Errorf("Tombstone contained object that is not an Autoscaler: %#v", obj)
		return
	}

	log.Debugf("Deleting Metrics from WPA %s/%s", deletedWPA.Namespace, deletedWPA.Name)
	toDelete := hpa.InspectWPA(deletedWPA)
	log.Debugf("Deleting %s/%s from the local cache", deletedWPA.Namespace, deletedWPA.Name)
	h.deleteFromLocalStore(toDelete)
	if err := h.store.DeleteExternalMetricValues(toDelete); err != nil {
		h.enqueue(deletedWPA)
		return
	}
}
