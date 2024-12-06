// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package listeners

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	infov1 "k8s.io/client-go/informers/core/v1"
	listv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	kubeEndpointsID   = "endpoints"
	kubeEndpointsName = "kube_endpoints"
	leaderAnnotation  = "control-plane.alpha.kubernetes.io/leader"
)

// KubeEndpointsListener listens to kubernetes endpoints creation
type KubeEndpointsListener struct {
	endpointsInformer  infov1.EndpointsInformer
	endpointsLister    listv1.EndpointsLister
	serviceInformer    infov1.ServiceInformer
	serviceLister      listv1.ServiceLister
	endpoints          map[k8stypes.UID][]*KubeEndpointService
	promInclAnnot      types.PrometheusAnnotations
	newService         chan<- Service
	delService         chan<- Service
	targetAllEndpoints bool
	m                  sync.RWMutex
	containerFilters   *containerFilters
	telemetryStore     *telemetry.Store
}

// KubeEndpointService represents an endpoint in a Kubernetes Endpoints
type KubeEndpointService struct {
	entity          string
	tags            []string
	hosts           map[string]string
	ports           []ContainerPort
	metricsExcluded bool
	globalExcluded  bool
}

// Make sure KubeEndpointService implements the Service interface
var _ Service = &KubeEndpointService{}

// NewKubeEndpointsListener returns the kube endpoints implementation of the ServiceListener interface
func NewKubeEndpointsListener(options ServiceListernerDeps) (ServiceListener, error) {
	// Using GetAPIClient (no wait) as Client should already be initialized by Cluster Agent main entrypoint before
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	endpointsInformer := ac.InformerFactory.Core().V1().Endpoints()
	if endpointsInformer == nil {
		return nil, fmt.Errorf("cannot get endpoints informer: %s", err)
	}

	serviceInformer := ac.InformerFactory.Core().V1().Services()
	if serviceInformer == nil {
		return nil, fmt.Errorf("cannot get service informer: %s", err)
	}

	containerFilters, err := newContainerFilters()
	if err != nil {
		return nil, err
	}

	return &KubeEndpointsListener{
		endpoints:          make(map[k8stypes.UID][]*KubeEndpointService),
		endpointsInformer:  endpointsInformer,
		endpointsLister:    endpointsInformer.Lister(),
		serviceInformer:    serviceInformer,
		serviceLister:      serviceInformer.Lister(),
		promInclAnnot:      getPrometheusIncludeAnnotations(),
		targetAllEndpoints: options.Config.IsProviderEnabled(names.KubeEndpointsFileRegisterName),
		containerFilters:   containerFilters,
		telemetryStore:     options.Telemetry,
	}, nil
}

// Listen starts watching service and endpoint events
func (l *KubeEndpointsListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	if _, err := l.endpointsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    l.endpointsAdded,
		DeleteFunc: l.endpointsDeleted,
		UpdateFunc: l.endpointsUpdated,
	}); err != nil {
		log.Errorf("cannot add event handler to endpoints informer: %s", err)
	}

	if _, err := l.serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: l.serviceUpdated,
	}); err != nil {
		log.Errorf("cannot add event handler to service informer: %s", err)
	}

	// Initial fill
	endpoints, err := l.endpointsLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list Kubernetes endpoints: %s", err)
	}
	for _, e := range endpoints {
		l.createService(e, true)
	}
}

// Stop is a stub
func (l *KubeEndpointsListener) Stop() {
	// We cannot deregister from the informer
}

func (l *KubeEndpointsListener) endpointsAdded(obj interface{}) {
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an *v1.Endpoints type, got: %T", obj)
		return
	}
	if isLockForLE(castedObj) {
		// Ignore Endpoints objects used for leader election
		return
	}
	l.createService(castedObj, true)
}

func (l *KubeEndpointsListener) endpointsDeleted(obj interface{}) {
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		// It's possible that we got a DeletedFinalStateUnknown here
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Errorf("Received unexpected object: %T", obj)
			return
		}

		castedObj, ok = deletedState.Obj.(*v1.Endpoints)
		if !ok {
			log.Errorf("Expected DeletedFinalStateUnknown to contain *v1.Endpoints, got: %T", deletedState.Obj)
			return
		}
	}
	if isLockForLE(castedObj) {
		// Ignore Endpoints objects used for leader election
		return
	}
	l.removeService(castedObj)
}

func (l *KubeEndpointsListener) endpointsUpdated(old, obj interface{}) {
	// Cast the updated object or return on failure
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an *v1.Endpoints type, got: %T", obj)
		return
	}
	if isLockForLE(castedObj) {
		// Ignore Endpoints objects used for leader election
		return
	}
	// Cast the old object, consider it an add on cast failure
	castedOld, ok := old.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an *v1.Endpoints type, got: %T", old)
		l.createService(castedObj, true)
		return
	}
	if l.endpointsDiffer(castedObj, castedOld) {
		l.removeService(castedObj)
		l.createService(castedObj, true)
	}
}

func (l *KubeEndpointsListener) serviceUpdated(old, obj interface{}) {
	// Cast the updated object or return on failure
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a *v1.Service type, got: %T", obj)
		return
	}

	// Cast the old object, consider it an add on cast failure
	castedOld, ok := old.(*v1.Service)
	if !ok {
		log.Errorf("Expected a *v1.Service type, got: %T", old)
		l.createService(l.endpointsForService(castedObj), false)
		return
	}

	// Detect if new annotations are added
	if !isServiceAnnotated(castedOld, kubeEndpointsID) && isServiceAnnotated(castedObj, kubeEndpointsID) {
		l.createService(l.endpointsForService(castedObj), false)
	}

	// Detect changes of AD labels for standard tags if the Service is annotated
	if isServiceAnnotated(castedObj, kubeEndpointsID) && (standardTagsDigest(castedOld.GetLabels()) != standardTagsDigest(castedObj.GetLabels())) {
		kep := l.endpointsForService(castedObj)
		l.removeService(kep)
		l.createService(kep, false)
	}
}

func (l *KubeEndpointsListener) endpointsForService(service *v1.Service) *v1.Endpoints {
	kendpoints, err := l.endpointsLister.Endpoints(service.Namespace).Get(service.Name)
	if err != nil {
		log.Warnf("Cannot get Kubernetes endpoints - Endpoints services won't be created - error: %s", err)
		return nil
	}

	return kendpoints
}

// endpointsDiffer compares two endpoints to only go forward
// when relevant fields are changed. This logic must be
// updated if more fields are used.
func (l *KubeEndpointsListener) endpointsDiffer(first, second *v1.Endpoints) bool {
	// Quick exit if resversion did not change
	if first.ResourceVersion == second.ResourceVersion {
		return false
	}

	ksvcFirst, err := l.serviceLister.Services(first.Namespace).Get(first.Name)
	if err != nil {
		log.Tracef("Cannot get Kubernetes service: %s", err)
		return true
	}

	ksvcSecond, err := l.serviceLister.Services(second.Namespace).Get(second.Name)
	if err != nil {
		log.Tracef("Cannot get Kubernetes service: %s", err)
		return true
	}

	// AD annotations on the corresponding services
	if isServiceAnnotated(ksvcFirst, kubeEndpointsID) != isServiceAnnotated(ksvcSecond, kubeEndpointsID) {
		return true
	}

	// AD labels - standard tags on the corresponding services
	if standardTagsDigest(ksvcFirst.GetLabels()) != standardTagsDigest(ksvcSecond.GetLabels()) {
		return true
	}

	// Endpoint subsets
	return subsetsDiffer(first, second)
}

// subsetsDiffer detects if two Endpoints have different subsets.
// The function is separated from endpointsDiffer to facilitate testing.
func subsetsDiffer(first, second *v1.Endpoints) bool {
	return !equality.Semantic.DeepEqual(first.Subsets, second.Subsets)
}

// isEndpointsAnnotated looks for the corresponding service of a kubernetes endpoints object
// and returns true if the service has endpoints annotations, otherwise returns false.
func (l *KubeEndpointsListener) isEndpointsAnnotated(kep *v1.Endpoints) bool {
	ksvc, err := l.serviceLister.Services(kep.Namespace).Get(kep.Name)
	if err != nil {
		log.Tracef("Cannot get Kubernetes service: %s", err)
		return false
	}
	return isServiceAnnotated(ksvc, kubeEndpointsID) || l.promInclAnnot.IsMatchingAnnotations(ksvc.GetAnnotations())
}

func (l *KubeEndpointsListener) shouldIgnore(kep *v1.Endpoints) bool {
	if l.targetAllEndpoints {
		return false
	}

	return !l.isEndpointsAnnotated(kep)
}

func (l *KubeEndpointsListener) createService(kep *v1.Endpoints, checkServiceAnnotations bool) {
	if kep == nil {
		return
	}

	if checkServiceAnnotations && l.shouldIgnore(kep) {
		// Ignore endpoints with no AD annotation on their corresponding service if checkServiceAnnotations
		// Typically we are called with checkServiceAnnotations = false when updates are due to changes on Kube Service object
		return
	}

	// Look for standard tags
	tags, err := l.getStandardTagsForEndpoints(kep)
	if err != nil {
		log.Debugf("Couldn't get standard tags for %s/%s: %v", kep.Namespace, kep.Name, err)
		tags = []string{}
	}

	eps := processEndpoints(kep, tags)

	for i := 0; i < len(eps); i++ {
		if l.containerFilters == nil {
			eps[i].metricsExcluded = false
			eps[i].globalExcluded = false
			continue
		}
		eps[i].metricsExcluded = l.containerFilters.IsExcluded(
			containers.MetricsFilter,
			kep.GetAnnotations(),
			kep.Name,
			"",
			kep.Namespace,
		)
		eps[i].globalExcluded = l.containerFilters.IsExcluded(
			containers.GlobalFilter,
			kep.GetAnnotations(),
			kep.Name,
			"",
			kep.Namespace,
		)
	}

	l.m.Lock()
	l.endpoints[kep.UID] = eps
	l.m.Unlock()
	telemetryStorePresent := l.telemetryStore != nil

	if telemetryStorePresent {
		l.telemetryStore.WatchedResources.Inc(kubeEndpointsName, telemetry.ResourceKubeService)
	}

	for _, ep := range eps {
		log.Debugf("Creating a new AD service: %s", ep.entity)
		l.newService <- ep
		if telemetryStorePresent {
			l.telemetryStore.WatchedResources.Inc(kubeEndpointsName, telemetry.ResourceKubeEndpoint)
		}
	}
}

// processEndpoints parses a kubernetes Endpoints object
// and returns a slice of KubeEndpointService per endpoint
func processEndpoints(kep *v1.Endpoints, tags []string) []*KubeEndpointService {
	var eps []*KubeEndpointService
	for i := range kep.Subsets {
		ports := []ContainerPort{}
		// Ports
		for _, port := range kep.Subsets[i].Ports {
			ports = append(ports, ContainerPort{int(port.Port), port.Name})
		}
		// Hosts
		for _, host := range kep.Subsets[i].Addresses {
			// create a separate AD service per host
			ep := &KubeEndpointService{
				entity: apiserver.EntityForEndpoints(kep.Namespace, kep.Name, host.IP),
				hosts:  map[string]string{"endpoint": host.IP},
				ports:  ports,
				tags: []string{
					fmt.Sprintf("kube_service:%s", kep.Name),
					fmt.Sprintf("kube_namespace:%s", kep.Namespace),
					fmt.Sprintf("kube_endpoint_ip:%s", host.IP),
				},
			}
			ep.tags = append(ep.tags, tags...)
			eps = append(eps, ep)
		}
	}
	return eps
}

func (l *KubeEndpointsListener) removeService(kep *v1.Endpoints) {
	if kep == nil {
		return
	}
	l.m.RLock()
	eps, ok := l.endpoints[kep.UID]
	l.m.RUnlock()
	if ok {
		l.m.Lock()
		delete(l.endpoints, kep.UID)
		l.m.Unlock()

		telemetryStorePresent := l.telemetryStore != nil

		if telemetryStorePresent {
			l.telemetryStore.WatchedResources.Dec(kubeEndpointsName, telemetry.ResourceKubeService)
		}

		for _, ep := range eps {
			log.Debugf("Deleting AD service: %s", ep.entity)
			l.delService <- ep
			if telemetryStorePresent {
				l.telemetryStore.WatchedResources.Dec(kubeEndpointsName, telemetry.ResourceKubeEndpoint)
			}
		}
	} else {
		log.Debugf("Entity %s not found, not removing", kep.UID)
	}
}

// isLockForLE returns true if the Endpoints object is used for leader election.
func isLockForLE(kep *v1.Endpoints) bool {
	if kep != nil {
		if _, found := kep.GetAnnotations()[leaderAnnotation]; found {
			return true
		}
	}
	return false
}

// getStandardTagsForEndpoints returns the standard tags defined in the labels
// of the Service that corresponds to a given Endpoints object.
func (l *KubeEndpointsListener) getStandardTagsForEndpoints(kep *v1.Endpoints) ([]string, error) {
	ksvc, err := l.serviceLister.Services(kep.Namespace).Get(kep.Name)
	if err != nil {
		return nil, err
	}
	return getStandardTags(ksvc.GetLabels()), nil
}

// Equal returns whether the two KubeEndpointService are equal
func (s *KubeEndpointService) Equal(o Service) bool {
	s2, ok := o.(*KubeEndpointService)
	if !ok {
		return false
	}

	return s.entity == s2.entity &&
		reflect.DeepEqual(s.tags, s2.tags) &&
		reflect.DeepEqual(s.hosts, s2.hosts) &&
		reflect.DeepEqual(s.ports, s2.ports)
}

// GetServiceID returns the unique entity name linked to that service
func (s *KubeEndpointService) GetServiceID() string {
	return s.entity
}

// GetADIdentifiers returns the service AD identifiers
func (s *KubeEndpointService) GetADIdentifiers(context.Context) ([]string, error) {
	return []string{s.entity}, nil
}

// GetHosts returns the pod hosts
func (s *KubeEndpointService) GetHosts(context.Context) (map[string]string, error) {
	if s.hosts == nil {
		return map[string]string{}, nil
	}
	return s.hosts, nil
}

// GetPid is not supported
func (s *KubeEndpointService) GetPid(context.Context) (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the endpoint's ports
func (s *KubeEndpointService) GetPorts(context.Context) ([]ContainerPort, error) {
	if s.ports == nil {
		return []ContainerPort{}, nil
	}
	return s.ports, nil
}

// GetTags retrieves tags
func (s *KubeEndpointService) GetTags() ([]string, error) {
	if s.tags == nil {
		return []string{}, nil
	}
	return s.tags, nil
}

// GetTagsWithCardinality returns the tags with given cardinality.
func (s *KubeEndpointService) GetTagsWithCardinality(_ string) ([]string, error) {
	return s.GetTags()
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubeEndpointService) GetHostname(context.Context) (string, error) {
	return "", ErrNotSupported
}

// IsReady returns if the service is ready
func (s *KubeEndpointService) IsReady(context.Context) bool {
	return true
}

// HasFilter returns whether the kube endpoint should not collect certain metrics
// due to filtering applied.
func (s *KubeEndpointService) HasFilter(filter containers.FilterType) bool {
	switch filter {
	case containers.MetricsFilter:
		return s.metricsExcluded
	case containers.GlobalFilter:
		return s.globalExcluded
	default:
		return false
	}
}

// GetExtraConfig isn't supported
//
//nolint:revive // TODO(CINT) Fix revive linter
func (s *KubeEndpointService) GetExtraConfig(key string) (string, error) {
	return "", ErrNotSupported
}

// FilterTemplates does nothing.
func (s *KubeEndpointService) FilterTemplates(map[string]integration.Config) {
}
