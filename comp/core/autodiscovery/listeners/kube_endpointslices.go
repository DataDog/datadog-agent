// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package listeners

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	infov1 "k8s.io/client-go/informers/core/v1"
	discinformers "k8s.io/client-go/informers/discovery/v1"
	listv1 "k8s.io/client-go/listers/core/v1"
	disclisters "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	kubeEndpointSlicesID       = "endpointslices"
	kubeEndpointSlicesName     = "kube_endpointslices"
	kubernetesServiceNameLabel = "kubernetes.io/service-name"
)

// KubeEndpointSlicesListener listens to kubernetes endpointslices creation
type KubeEndpointSlicesListener struct {
	endpointSliceInformer discinformers.EndpointSliceInformer
	endpointSliceLister   disclisters.EndpointSliceLister
	serviceInformer       infov1.ServiceInformer
	serviceLister         listv1.ServiceLister

	// Storage: Key is "namespace/serviceName" to aggregate all slices per service
	services map[string][]*KubeEndpointService
	// Helper to map slice UID to service key for deletion handling
	sliceToService map[k8stypes.UID]string

	promInclAnnot      types.PrometheusAnnotations
	newService         chan<- Service
	delService         chan<- Service
	targetAllEndpoints bool
	m                  sync.RWMutex
	filterStore        workloadfilter.Component
	telemetryStore     *telemetry.Store
}

// NewKubeEndpointSlicesListener returns the kube endpointslices implementation of the ServiceListener interface
func NewKubeEndpointSlicesListener(options ServiceListernerDeps) (ServiceListener, error) {
	// Using GetAPIClient (no wait) as Client should already be initialized by Cluster Agent main entrypoint before
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	endpointSliceInformer := ac.InformerFactory.Discovery().V1().EndpointSlices()
	if endpointSliceInformer == nil {
		return nil, fmt.Errorf("cannot get endpointslice informer: %s", err)
	}

	serviceInformer := ac.InformerFactory.Core().V1().Services()
	if serviceInformer == nil {
		return nil, fmt.Errorf("cannot get service informer: %s", err)
	}

	return &KubeEndpointSlicesListener{
		services:              make(map[string][]*KubeEndpointService),
		sliceToService:        make(map[k8stypes.UID]string),
		endpointSliceInformer: endpointSliceInformer,
		endpointSliceLister:   endpointSliceInformer.Lister(),
		serviceInformer:       serviceInformer,
		serviceLister:         serviceInformer.Lister(),
		promInclAnnot:         getPrometheusIncludeAnnotations(),
		targetAllEndpoints:    options.Config.IsProviderEnabled(names.KubeEndpointsFileRegisterName),
		filterStore:           options.Filter,
		telemetryStore:        options.Telemetry,
	}, nil
}

// Listen starts watching service and endpointslice events
func (l *KubeEndpointSlicesListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	if _, err := l.endpointSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    l.endpointSliceAdded,
		DeleteFunc: l.endpointSliceDeleted,
		UpdateFunc: l.endpointSliceUpdated,
	}); err != nil {
		log.Errorf("cannot add event handler to endpointslice informer: %s", err)
	}

	if _, err := l.serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: l.serviceUpdated,
	}); err != nil {
		log.Errorf("cannot add event handler to service informer: %s", err)
	}

	// Initial fill
	endpointSlices, err := l.endpointSliceLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list Kubernetes endpointslices: %s", err)
	}

	// Group slices by service and process
	serviceSlices := make(map[string][]*discv1.EndpointSlice)
	for _, slice := range endpointSlices {
		serviceName, ok := slice.Labels[kubernetesServiceNameLabel]
		if !ok || serviceName == "" {
			continue
		}
		serviceKey := fmt.Sprintf("%s/%s", slice.Namespace, serviceName)
		serviceSlices[serviceKey] = append(serviceSlices[serviceKey], slice)
	}

	for serviceKey, slices := range serviceSlices {
		l.createServiceFromSlices(serviceKey, slices, true)
	}
}

// Stop is a stub
func (l *KubeEndpointSlicesListener) Stop() {
	// We cannot deregister from the informer
}

func (l *KubeEndpointSlicesListener) endpointSliceAdded(obj interface{}) {
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an *discv1.EndpointSlice type, got: %T", obj)
		return
	}
	l.processEndpointSliceChange(slice)
}

func (l *KubeEndpointSlicesListener) endpointSliceDeleted(obj interface{}) {
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		// It's possible that we got a DeletedFinalStateUnknown here
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Errorf("Received unexpected object: %T", obj)
			return
		}

		slice, ok = deletedState.Obj.(*discv1.EndpointSlice)
		if !ok {
			log.Errorf("Expected DeletedFinalStateUnknown to contain *discv1.EndpointSlice, got: %T", deletedState.Obj)
			return
		}
	}
	l.processEndpointSliceChange(slice)
}

func (l *KubeEndpointSlicesListener) endpointSliceUpdated(old, obj interface{}) {
	// Cast the updated object or return on failure
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an *discv1.EndpointSlice type, got: %T", obj)
		return
	}

	// Cast the old object, consider it an add on cast failure
	oldSlice, ok := old.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an *discv1.EndpointSlice type, got: %T", old)
		l.processEndpointSliceChange(slice)
		return
	}

	if l.endpointSlicesDiffer(slice, oldSlice) {
		l.processEndpointSliceChange(slice)
	}
}

func (l *KubeEndpointSlicesListener) serviceUpdated(old, obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a *v1.Service type, got: %T", obj)
		return
	}

	// Cast the old object, consider it an add on cast failure
	oldSvc, ok := old.(*v1.Service)
	if !ok {
		log.Errorf("Expected a *v1.Service type, got: %T", old)
		l.processServiceUpdate(svc)
		return
	}

	// Detect if new annotations are added
	if !isServiceAnnotated(oldSvc, kubeEndpointSlicesID) && isServiceAnnotated(svc, kubeEndpointSlicesID) {
		l.processServiceUpdate(svc)
	}

	// Detect changes of AD labels for standard tags if the Service is annotated
	if isServiceAnnotated(svc, kubeEndpointSlicesID) &&
		(standardTagsDigest(oldSvc.GetLabels()) != standardTagsDigest(svc.GetLabels())) {
		l.processServiceUpdate(svc)
	}
}

// processEndpointSliceChange handles add/update/delete events for a single endpointslice
// by fetching all slices for the service and regenerating the service
func (l *KubeEndpointSlicesListener) processEndpointSliceChange(slice *discv1.EndpointSlice) {
	serviceName, ok := slice.Labels[kubernetesServiceNameLabel]
	if !ok || serviceName == "" {
		log.Tracef("EndpointSlice %s/%s missing %s label; skipping", slice.Namespace, slice.Name, kubernetesServiceNameLabel)
		return
	}

	serviceKey := fmt.Sprintf("%s/%s", slice.Namespace, serviceName)

	allSlices, err := l.endpointSliceLister.EndpointSlices(slice.Namespace).List(
		labels.Set{kubernetesServiceNameLabel: serviceName}.AsSelector(),
	)
	if err != nil {
		log.Errorf("Cannot list EndpointSlices for service %s: %s", serviceKey, err)
		return
	}

	l.removeServiceByKey(serviceKey)
	l.createServiceFromSlices(serviceKey, allSlices, true)
}

// processServiceUpdate handles service update events by regenerating endpoints
func (l *KubeEndpointSlicesListener) processServiceUpdate(svc *v1.Service) {
	serviceKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)

	allSlices, err := l.endpointSliceLister.EndpointSlices(svc.Namespace).List(
		labels.Set{kubernetesServiceNameLabel: svc.Name}.AsSelector(),
	)
	if err != nil {
		log.Warnf("Cannot get Kubernetes endpointslices - EndpointSlice services won't be created - error: %s", err)
		return
	}

	l.removeServiceByKey(serviceKey)
	l.createServiceFromSlices(serviceKey, allSlices, false)
}

// endpointSlicesDiffer compares two endpointslices to only go forward
// when relevant fields are changed.
func (l *KubeEndpointSlicesListener) endpointSlicesDiffer(first, second *discv1.EndpointSlice) bool {
	// Quick exit if resource version did not change
	if first.ResourceVersion == second.ResourceVersion {
		return false
	}

	serviceName := first.Labels[kubernetesServiceNameLabel]
	if serviceName == "" {
		return true
	}

	ksvc, err := l.serviceLister.Services(first.Namespace).Get(serviceName)
	if err != nil {
		log.Tracef("Cannot get Kubernetes service: %s", err)
		return true
	}

	ksvcSecond, err := l.serviceLister.Services(second.Namespace).Get(serviceName)
	if err != nil {
		log.Tracef("Cannot get Kubernetes service: %s", err)
		return true
	}

	// AD annotations on the corresponding services
	if isServiceAnnotated(ksvc, kubeEndpointSlicesID) != isServiceAnnotated(ksvcSecond, kubeEndpointSlicesID) {
		return true
	}

	// AD labels - standard tags on the corresponding services
	if standardTagsDigest(ksvc.GetLabels()) != standardTagsDigest(ksvcSecond.GetLabels()) {
		return true
	}

	// Check if endpoints themselves differ
	return endpointSliceEndpointsDiffer(first, second)
}

// endpointSliceEndpointsDiffer compares the endpoints in two endpointslices
func endpointSliceEndpointsDiffer(first, second *discv1.EndpointSlice) bool {
	if len(first.Endpoints) != len(second.Endpoints) {
		return true
	}

	// Simple comparison - could be optimized with a more sophisticated diff
	for i := range first.Endpoints {
		if len(first.Endpoints[i].Addresses) != len(second.Endpoints[i].Addresses) {
			return true
		}
		for j := range first.Endpoints[i].Addresses {
			if first.Endpoints[i].Addresses[j] != second.Endpoints[i].Addresses[j] {
				return true
			}
		}
	}

	return false
}

// isEndpointSlicesAnnotated looks for the corresponding service of a kubernetes endpointslice object
// and returns true if the service has endpointslices annotations, otherwise returns false.
func (l *KubeEndpointSlicesListener) isEndpointSlicesAnnotated(slice *discv1.EndpointSlice) bool {
	serviceName := slice.Labels[kubernetesServiceNameLabel]
	if serviceName == "" {
		return false
	}

	ksvc, err := l.serviceLister.Services(slice.Namespace).Get(serviceName)
	if err != nil {
		log.Tracef("Cannot get Kubernetes service: %s", err)
		return false
	}
	return isServiceAnnotated(ksvc, kubeEndpointSlicesID) || l.promInclAnnot.IsMatchingAnnotations(ksvc.GetAnnotations())
}

func (l *KubeEndpointSlicesListener) shouldIgnore(slice *discv1.EndpointSlice) bool {
	if l.targetAllEndpoints {
		return false
	}

	return !l.isEndpointSlicesAnnotated(slice)
}

func (l *KubeEndpointSlicesListener) createServiceFromSlices(serviceKey string, slices []*discv1.EndpointSlice, checkServiceAnnotations bool) {
	if len(slices) == 0 {
		return
	}

	// All slices for a service share namespace and service name
	namespace := slices[0].Namespace
	serviceName := slices[0].Labels[kubernetesServiceNameLabel]

	if checkServiceAnnotations && l.shouldIgnore(slices[0]) {
		// Ignore endpointslices with no AD annotation on their corresponding service if checkServiceAnnotations
		return
	}

	// Look for standard tags from the service
	tags, err := l.getStandardTagsForService(namespace, serviceName)
	if err != nil {
		log.Debugf("Couldn't get standard tags for %s: %v", serviceKey, err)
		tags = []string{}
	}

	eps := processEndpointSlices(slices, tags, l.filterStore)

	l.m.Lock()
	l.services[serviceKey] = eps
	// Track all slices for this service
	for _, slice := range slices {
		l.sliceToService[slice.UID] = serviceKey
	}
	l.m.Unlock()

	telemetryStorePresent := l.telemetryStore != nil

	if telemetryStorePresent {
		l.telemetryStore.WatchedResources.Inc(kubeEndpointSlicesName, telemetry.ResourceKubeService)
	}

	for _, ep := range eps {
		log.Debugf("Creating a new AD service: %s", ep.entity)
		l.newService <- ep
		if telemetryStorePresent {
			l.telemetryStore.WatchedResources.Inc(kubeEndpointSlicesName, telemetry.ResourceKubeEndpoint)
		}
	}
}

// processEndpointSlices parses kubernetes EndpointSlice objects
// and returns a slice of KubeEndpointService per endpoint IP
func processEndpointSlices(slices []*discv1.EndpointSlice, tags []string, filterStore workloadfilter.Component) []*KubeEndpointService {
	var eps []*KubeEndpointService

	if len(slices) == 0 {
		return eps
	}

	// Extract service name and namespace from first slice (same for all)
	serviceName := slices[0].Labels[kubernetesServiceNameLabel]
	namespace := slices[0].Namespace

	filterableEndpoint := workloadfilter.CreateKubeEndpoint(serviceName, namespace, slices[0].GetAnnotations())
	metricsExcluded := filterStore.GetKubeEndpointAutodiscoveryFilters(workloadfilter.MetricsFilter).IsExcluded(filterableEndpoint)
	globalExcluded := filterStore.GetKubeEndpointAutodiscoveryFilters(workloadfilter.GlobalFilter).IsExcluded(filterableEndpoint)

	for _, slice := range slices {
		ports := []workloadmeta.ContainerPort{}
		for _, port := range slice.Ports {
			if port.Port != nil && port.Name != nil {
				ports = append(ports, workloadmeta.ContainerPort{Port: int(*port.Port), Name: *port.Name})
			}
		}

		// Iterate through endpoints (IP addresses)
		for _, endpoint := range slice.Endpoints {
			for _, ip := range endpoint.Addresses {
				ep := &KubeEndpointService{
					entity:   apiserver.EntityForEndpoints(namespace, serviceName, ip),
					metadata: filterableEndpoint,
					hosts:    map[string]string{"endpoint": ip},
					ports:    ports,
					tags: []string{
						"kube_service:" + serviceName,
						"kube_namespace:" + namespace,
						"kube_endpoint_ip:" + ip,
					},
					metricsExcluded: metricsExcluded,
					globalExcluded:  globalExcluded,
					namespace:       namespace,
				}
				ep.tags = append(ep.tags, tags...)
				eps = append(eps, ep)
			}
		}
	}
	return eps
}

func (l *KubeEndpointSlicesListener) removeServiceByKey(serviceKey string) {
	l.m.RLock()
	eps, ok := l.services[serviceKey]
	l.m.RUnlock()

	if ok {
		l.m.Lock()
		delete(l.services, serviceKey)
		for sliceUID, svcKey := range l.sliceToService {
			if svcKey == serviceKey {
				delete(l.sliceToService, sliceUID)
			}
		}
		l.m.Unlock()

		telemetryStorePresent := l.telemetryStore != nil

		if telemetryStorePresent {
			l.telemetryStore.WatchedResources.Dec(kubeEndpointSlicesName, telemetry.ResourceKubeService)
		}

		for _, ep := range eps {
			log.Debugf("Deleting AD service: %s", ep.entity)
			l.delService <- ep
			if telemetryStorePresent {
				l.telemetryStore.WatchedResources.Dec(kubeEndpointSlicesName, telemetry.ResourceKubeEndpoint)
			}
		}
	} else {
		log.Debugf("Service %s not found, not removing", serviceKey)
	}
}

// getStandardTagsForService returns the standard tags defined in the labels
// of a given Service.
func (l *KubeEndpointSlicesListener) getStandardTagsForService(namespace, serviceName string) ([]string, error) {
	ksvc, err := l.serviceLister.Services(namespace).Get(serviceName)
	if err != nil {
		return nil, err
	}
	return getStandardTags(ksvc.GetLabels()), nil
}
