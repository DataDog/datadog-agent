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
	"k8s.io/apimachinery/pkg/api/equality"

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
	kubeEndpointSlicesID       = "endpoints"
	kubeEndpointSlicesName     = "kube_endpointslices"
	kubernetesServiceNameLabel = "kubernetes.io/service-name"
)

// KubeEndpointSlicesListener listens to kubernetes endpointslices creation
type KubeEndpointSlicesListener struct {
	endpointSliceInformer discinformers.EndpointSliceInformer
	endpointSliceLister   disclisters.EndpointSliceLister
	serviceInformer       infov1.ServiceInformer
	serviceLister         listv1.ServiceLister

	// Storage: Track endpoints per slice (not per service) for granular updates
	// Key is EndpointSlice UID -> endpoints from that specific slice
	endpointsBySlice map[k8stypes.UID][]*KubeEndpointService
	// Helper to map slice UID to service key for service-level operations
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
		endpointsBySlice:      make(map[k8stypes.UID][]*KubeEndpointService),
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

	endpointSlices, err := l.endpointSliceLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list Kubernetes endpointslices: %s", err)
	}

	for _, slice := range endpointSlices {
		l.createServiceFromSlice(slice, true)
	}
}

func (l *KubeEndpointSlicesListener) Stop() {
	// We cannot deregister from the informer
}

func (l *KubeEndpointSlicesListener) endpointSliceAdded(obj interface{}) {
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an *discv1.EndpointSlice type, got: %T", obj)
		return
	}
	l.createServiceFromSlice(slice, true)
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
	l.removeServiceFromSlice(slice)
}

func (l *KubeEndpointSlicesListener) endpointSliceUpdated(old, obj interface{}) {
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an *discv1.EndpointSlice type, got: %T", obj)
		return
	}

	// Cast the old object, consider it an add on cast failure
	oldSlice, ok := old.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an *discv1.EndpointSlice type, got: %T", old)
		l.createServiceFromSlice(slice, true)
		return
	}

	if l.endpointSlicesDiffer(slice, oldSlice) {
		l.removeServiceFromSlice(oldSlice)
		l.createServiceFromSlice(slice, true)
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

// createServiceFromSlice processes a single EndpointSlice and creates KubeEndpointServices for its endpoints
func (l *KubeEndpointSlicesListener) createServiceFromSlice(slice *discv1.EndpointSlice, checkServiceAnnotations bool) {
	if slice == nil {
		return
	}

	serviceName, ok := slice.Labels[kubernetesServiceNameLabel]
	if !ok || serviceName == "" {
		log.Tracef("EndpointSlice %s/%s missing %s label; skipping", slice.Namespace, slice.Name, kubernetesServiceNameLabel)
		return
	}

	if checkServiceAnnotations && l.shouldIgnore(slice) {
		// Ignore endpointslices with no AD annotation on their corresponding service
		return
	}

	serviceKey := fmt.Sprintf("%s/%s", slice.Namespace, serviceName)

	// Look for standard tags from the service
	tags, err := l.getStandardTagsForService(slice.Namespace, serviceName)
	if err != nil {
		log.Debugf("Couldn't get standard tags for %s: %v", serviceKey, err)
		tags = []string{}
	}

	eps := processEndpointSlice(slice, tags, l.filterStore)

	l.m.Lock()
	// Store endpoints by slice UID for granular updates
	oldEps := l.endpointsBySlice[slice.UID]
	l.endpointsBySlice[slice.UID] = eps
	l.sliceToService[slice.UID] = serviceKey
	l.m.Unlock()

	telemetryStorePresent := l.telemetryStore != nil

	// Only increment service counter if this is the first slice for this service
	if len(oldEps) == 0 && len(eps) > 0 && telemetryStorePresent {
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

// removeServiceFromSlice removes all endpoints from a specific slice
func (l *KubeEndpointSlicesListener) removeServiceFromSlice(slice *discv1.EndpointSlice) {
	if slice == nil {
		return
	}

	l.m.RLock()
	eps, ok := l.endpointsBySlice[slice.UID]
	l.m.RUnlock()

	if ok {
		l.m.Lock()
		delete(l.endpointsBySlice, slice.UID)
		delete(l.sliceToService, slice.UID)
		l.m.Unlock()

		telemetryStorePresent := l.telemetryStore != nil

		for _, ep := range eps {
			log.Debugf("Deleting AD service from slice deletion: %s", ep.entity)
			l.delService <- ep
			if telemetryStorePresent {
				l.telemetryStore.WatchedResources.Dec(kubeEndpointSlicesName, telemetry.ResourceKubeEndpoint)
			}
		}
	} else {
		log.Debugf("EndpointSlice %s not found, not removing", slice.UID)
	}
}

// processServiceUpdate handles service update events by regenerating all endpoints for the service
// This is only called when service annotations/labels change, which affects ALL endpoints
func (l *KubeEndpointSlicesListener) processServiceUpdate(svc *v1.Service) {
	allSlices, err := l.endpointSliceLister.EndpointSlices(svc.Namespace).List(
		labels.Set{kubernetesServiceNameLabel: svc.Name}.AsSelector(),
	)
	if err != nil {
		log.Warnf("Cannot get Kubernetes endpointslices - EndpointSlice services won't be updated - error: %s", err)
		return
	}

	// When service metadata changes, we need to update ALL slices
	// Remove and recreate all endpoints for this service
	for _, slice := range allSlices {
		l.removeServiceFromSlice(slice)
		l.createServiceFromSlice(slice, false)
	}
}

// endpointSlicesDiffer compares two endpointslices to only go forward
// when relevant fields are changed.
func (l *KubeEndpointSlicesListener) endpointSlicesDiffer(first, second *discv1.EndpointSlice) bool {
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

	return !equality.Semantic.DeepEqual(first, second)
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

// processEndpointSlice parses a single kubernetes EndpointSlice object
// and returns a slice of KubeEndpointService per endpoint IP
func processEndpointSlice(slice *discv1.EndpointSlice, tags []string, filterStore workloadfilter.Component) []*KubeEndpointService {
	var eps []*KubeEndpointService

	if slice == nil {
		return eps
	}

	// Extract service name and namespace
	serviceName := slice.Labels[kubernetesServiceNameLabel]
	namespace := slice.Namespace

	if serviceName == "" {
		return eps
	}

	filterableEndpoint := workloadfilter.CreateKubeEndpoint(serviceName, namespace, slice.GetAnnotations())
	metricsExcluded := filterStore.GetKubeEndpointAutodiscoveryFilters(workloadfilter.MetricsFilter).IsExcluded(filterableEndpoint)
	globalExcluded := filterStore.GetKubeEndpointAutodiscoveryFilters(workloadfilter.GlobalFilter).IsExcluded(filterableEndpoint)

	// Ports are at the slice level
	ports := []workloadmeta.ContainerPort{}
	for _, port := range slice.Ports {
		if port.Port != nil && port.Name != nil {
			ports = append(ports, workloadmeta.ContainerPort{
				Port: int(*port.Port),
				Name: *port.Name,
			})
		}
	}

	// Iterate through endpoints (IP addresses)
	for _, endpoint := range slice.Endpoints {
		// Extract pod metadata from TargetRef (if available)
		var podUID, nodeName string
		if endpoint.TargetRef != nil && endpoint.TargetRef.Kind == "Pod" {
			podUID = string(endpoint.TargetRef.UID)
		}
		if endpoint.NodeName != nil {
			nodeName = *endpoint.NodeName
		}

		for _, ip := range endpoint.Addresses {
			// Create a separate AD service per IP
			ep := &KubeEndpointService{
				entity:      apiserver.EntityForEndpoints(namespace, serviceName, ip),
				serviceName: serviceName,
				podUID:      podUID,
				nodeName:    nodeName,
				metadata:    filterableEndpoint,
				hosts:       map[string]string{"endpoint": ip},
				ports:       ports,
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
	return eps
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
