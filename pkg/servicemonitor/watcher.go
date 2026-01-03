package servicemonitor

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var gvr = schema.GroupVersionResource{
	Group:    "datadoghq.com",
	Version:  "v1alpha1",
	Resource: "datadogservicemonitors",
}

type ServiceMonitorWatcher struct {
	store                  *Store
	serviceMonitorLister   cache.GenericLister
	serviceMonitorSynced   cache.InformerSynced
	dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory
}

func NewServiceMonitorWatcher(store *Store, dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory) (*ServiceMonitorWatcher, error) {
	if store == nil {
		return nil, fmt.Errorf("Store must be initialized")
	}

	if dynamicInformerFactory == nil {
		return nil, fmt.Errorf("Dynamic informer factory must be initialized")
	}

	// Setup ServiceMonitor informer using the provided dynamic informer factory
	serviceMonitorInformer := dynamicInformerFactory.ForResource(gvr)
	serviceMonitorLister := serviceMonitorInformer.Lister()
	serviceMonitorSynced := serviceMonitorInformer.Informer().HasSynced

	watcher := &ServiceMonitorWatcher{
		store:                  store,
		serviceMonitorLister:   serviceMonitorLister,
		serviceMonitorSynced:   serviceMonitorSynced,
		dynamicInformerFactory: dynamicInformerFactory,
	}

	serviceMonitorInformer.Informer().SetWatchErrorHandler(func(r *cache.Reflector, err error) {
		log.Errorf("ServiceMonitor informer watch error: %v", err)
	})

	// Add event handlers
	if _, err := serviceMonitorInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    watcher.onAdd,
		UpdateFunc: watcher.onUpdate,
		DeleteFunc: watcher.onDelete,
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to ServiceMonitor informer: %s", err)
	}

	return watcher, nil
}

func (w *ServiceMonitorWatcher) Run(stopCh <-chan struct{}) {
	log.Infof("Starting ServiceMonitorWatcher")
	w.dynamicInformerFactory.Start(stopCh)
	log.Info("dynamicInformerFactory started")
	go w.run(stopCh)
	log.Infof("ServiceMonitorWatcher started")
}

func (w *ServiceMonitorWatcher) run(stopCh <-chan struct{}) {
	log.Infof("Starting ServiceMonitorWatcher (waiting for cache sync)")

	// Wait for cache sync
	if !cache.WaitForCacheSync(stopCh, w.serviceMonitorSynced) {
		log.Errorf("Failed to sync ServiceMonitor cache")
		return
	}

	log.Infof("ServiceMonitorWatcher started (cache sync finished)")

	// Perform initial fill from existing resources
	w.initialFill()

	// Wait for stop signal
	<-stopCh
	log.Infof("Stopping ServiceMonitorWatcher")
}

// initialFill populates the store with existing ServiceMonitor resources
func (w *ServiceMonitorWatcher) initialFill() {
	log.Infof("Initial fill of ServiceMonitor resources")
	serviceMonitors, err := w.serviceMonitorLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list ServiceMonitor resources: %s", err)
		return
	}

	for _, obj := range serviceMonitors {
		if unstructuredObj, ok := obj.(*unstructured.Unstructured); ok {
			w.processServiceMonitor(unstructuredObj, "initial-fill")
		}
	}
}

// onAdd handles ServiceMonitor creation events
func (w *ServiceMonitorWatcher) onAdd(obj interface{}) {
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		log.Errorf("Expected *unstructured.Unstructured, got: %T", obj)
		return
	}

	log.Infof("ServiceMonitor added: %s/%s", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
	w.processServiceMonitor(unstructuredObj, "add")
}

// onUpdate handles ServiceMonitor update events
func (w *ServiceMonitorWatcher) onUpdate(oldObj, newObj interface{}) {
	newUnstructuredObj, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		log.Errorf("Expected *unstructured.Unstructured, got: %T", newObj)
		return
	}

	log.Infof("ServiceMonitor updated: %s/%s", newUnstructuredObj.GetNamespace(), newUnstructuredObj.GetName())
	w.processServiceMonitor(newUnstructuredObj, "update")
}

// onDelete handles ServiceMonitor deletion events
func (w *ServiceMonitorWatcher) onDelete(obj interface{}) {
	var unstructuredObj *unstructured.Unstructured
	var ok bool

	unstructuredObj, ok = obj.(*unstructured.Unstructured)
	if !ok {
		// It's possible that we got a DeletedFinalStateUnknown here
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Errorf("Received unexpected object: %T", obj)
			return
		}

		unstructuredObj, ok = deletedState.Obj.(*unstructured.Unstructured)
		if !ok {
			log.Errorf("Expected DeletedFinalStateUnknown to contain *unstructured.Unstructured, got: %T", deletedState.Obj)
			return
		}
	}

	log.Debugf("ServiceMonitor deleted: %s/%s", unstructuredObj.GetNamespace(), unstructuredObj.GetName())

	// Generate the key for the deleted ServiceMonitor
	w.store.DeleteDatadogServiceMonitor(unstructuredObj.GetName())
}

// processServiceMonitor converts an unstructured ServiceMonitor to a DatadogServiceMonitor and updates the store
func (w *ServiceMonitorWatcher) processServiceMonitor(unstructuredObj *unstructured.Unstructured, operation string) {
	// Convert unstructured to DatadogServiceMonitor
	var serviceMonitor DatadogServiceMonitor
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.UnstructuredContent(), &serviceMonitor); err != nil {
		log.Errorf("Failed to convert unstructured ServiceMonitor to DatadogServiceMonitor: %s", err)
		return
	}

	// Generate a unique key for the ServiceMonitor
	key := fmt.Sprintf("%s/%s", serviceMonitor.Namespace, serviceMonitor.Name)

	// Set the name in the ServiceMonitor if not already set
	if serviceMonitor.Name == "" {
		serviceMonitor.Name = key
	}

	log.Infof("Processing ServiceMonitor %s (operation: %s)", key, operation)

	// Update the store
	w.store.SetDatadogServiceMonitor(serviceMonitor)
}
