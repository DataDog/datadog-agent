// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	providerTypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	discinfov1 "k8s.io/client-go/informers/discovery/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	disclistersv1 "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"
)

// ServiceEndpointSlicesAPI abstracts the dependency on the Kubernetes API (useful for testing)
type ServiceEndpointSlicesAPI interface {
	// ListServices lists all Services
	ListServices() ([]*v1.Service, error)
	// ListEndpointSlices lists EndpointSlices by namespace and service name
	ListEndpointSlices(namespace, name string) ([]*discv1.EndpointSlice, error)
}

type svcEndpointSlicesAPI struct {
	serviceLister       listersv1.ServiceLister
	endpointSliceLister disclistersv1.EndpointSliceLister
}

func (api *svcEndpointSlicesAPI) ListServices() ([]*v1.Service, error) {
	return api.serviceLister.List(labels.Everything())
}

func (api *svcEndpointSlicesAPI) ListEndpointSlices(namespace, name string) ([]*discv1.EndpointSlice, error) {
	return api.endpointSliceLister.EndpointSlices(namespace).List(
		labels.Set{apiserver.KubernetesServiceNameLabel: name}.AsSelector(),
	)
}

// PrometheusServicesEndpointSlicesConfigProvider implements the ConfigProvider interface for prometheus services using EndpointSlices
type PrometheusServicesEndpointSlicesConfigProvider struct {
	sync.RWMutex

	api      ServiceEndpointSlicesAPI
	upToDate bool

	collectEndpoints   bool
	monitoredEndpoints map[string]bool

	checks []*types.PrometheusCheck
}

// NewPrometheusServicesEndpointSlicesConfigProvider returns a new Prometheus ConfigProvider connected to kube apiserver using EndpointSlices
func NewPrometheusServicesEndpointSlicesConfigProvider(*pkgconfigsetup.ConfigurationProviders, *telemetry.Store) (providerTypes.ConfigProvider, error) {
	// Using GetAPIClient (no wait) as Client should already be initialized by Cluster Agent main entrypoint before
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	servicesInformer := ac.InformerFactory.Core().V1().Services()
	if servicesInformer == nil {
		return nil, errors.New("cannot get services informer")
	}

	var endpointSliceInformer discinfov1.EndpointSliceInformer
	var endpointSliceLister disclistersv1.EndpointSliceLister

	collectEndpoints := pkgconfigsetup.Datadog().GetBool("prometheus_scrape.service_endpoints")
	if collectEndpoints {
		endpointSliceInformer = ac.InformerFactory.Discovery().V1().EndpointSlices()
		if endpointSliceInformer == nil {
			return nil, errors.New("cannot get endpointslice informer")
		}
		endpointSliceLister = endpointSliceInformer.Lister()
	}

	api := &svcEndpointSlicesAPI{
		serviceLister:       servicesInformer.Lister(),
		endpointSliceLister: endpointSliceLister,
	}

	checks, err := getPrometheusConfigs()
	if err != nil {
		return nil, err
	}

	p := newPromServicesEndpointSlicesProvider(checks, api, collectEndpoints)

	if _, err := servicesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.invalidate,
		UpdateFunc: p.invalidateIfChanged,
		DeleteFunc: p.invalidate,
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to services informer: %s", err)
	}

	if endpointSliceInformer != nil {
		if _, err := endpointSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    p.invalidateIfAddedEndpointSlices,
			UpdateFunc: p.invalidateIfChangedEndpointSlices,
		}); err != nil {
			return nil, fmt.Errorf("cannot add event handler to endpointslice informer: %s", err)
		}
	}
	return p, nil
}

func newPromServicesEndpointSlicesProvider(checks []*types.PrometheusCheck, api ServiceEndpointSlicesAPI, collectEndpoints bool) *PrometheusServicesEndpointSlicesConfigProvider {
	return &PrometheusServicesEndpointSlicesConfigProvider{
		checks:             checks,
		api:                api,
		collectEndpoints:   collectEndpoints,
		monitoredEndpoints: make(map[string]bool),
	}
}

// String returns a string representation of the PrometheusServicesEndpointSlicesConfigProvider
func (p *PrometheusServicesEndpointSlicesConfigProvider) String() string {
	return names.PrometheusServicesEndpointSlices
}

// Collect retrieves services from the apiserver, builds Config objects and returns them
func (p *PrometheusServicesEndpointSlicesConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	services, err := p.api.ListServices()
	if err != nil {
		return nil, err
	}

	var configs []integration.Config
	for _, svc := range services {
		for _, check := range p.checks {
			if !check.IsIncluded(svc.Annotations) {
				log.Tracef("Service %s/%s does not have matching annotations, skipping", svc.Namespace, svc.Name)
				continue
			}

			if !p.collectEndpoints {
				// Only generates Service checks if EndpointSlice checks are not active
				serviceConfigs := utils.ConfigsForService(check, svc)

				if len(serviceConfigs) != 0 {
					configs = append(configs, serviceConfigs...)
				}
			} else {
				slices, err := p.api.ListEndpointSlices(svc.GetNamespace(), svc.GetName())
				if err != nil {
					return nil, err
				}

				if len(slices) == 0 {
					// No EndpointSlices found for this service, which can happen when
					// the service is headless/external or the service hasn't been assigned an endpoint yet.
					continue
				}

				// Add endpoint to tracking as soon as there are annotations (even if no config yet due to no endpoints)
				// Otherwise if `Collect` happens to run before EndpointSlice object has at least one target
				// It will be ignored forever.
				// Note: a race can still happen and delay the check scheduling for 5 minutes (first creation of the service)
				endpointsID := apiserver.EntityForEndpoints(svc.GetNamespace(), svc.GetName(), "")
				p.Lock()
				p.monitoredEndpoints[endpointsID] = true
				p.Unlock()

				// Generate ONE config per service (not per endpoint IP).
				// The config uses a service-level AD identifier that matches
				// all endpoint services created by the listener.
				serviceLevelConfig := utils.ConfigForServiceEndpointSlices(check, svc)
				if serviceLevelConfig != nil {
					configs = append(configs, *serviceLevelConfig)
				}
			}
		}
	}

	p.setUpToDate(true)
	return configs, nil
}

// setUpToDate is a thread-safe method to update the upToDate value
func (p *PrometheusServicesEndpointSlicesConfigProvider) setUpToDate(v bool) {
	p.Lock()
	defer p.Unlock()
	p.upToDate = v
}

// IsUpToDate allows to cache configs as long as no changes are detected in the apiserver
func (p *PrometheusServicesEndpointSlicesConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	p.RLock()
	defer p.RUnlock()
	return p.upToDate, nil
}

func (p *PrometheusServicesEndpointSlicesConfigProvider) invalidate(obj interface{}) {
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		// It's possible that we got a DeletedFinalStateUnknown here
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Errorf("Received unexpected object: %T", obj)
			return
		}

		castedObj, ok = deletedState.Obj.(*v1.Service)
		if !ok {
			log.Errorf("Expected DeletedFinalStateUnknown to contain *v1.Service, got: %T", deletedState.Obj)
			return
		}
	}
	p.Lock()
	defer p.Unlock()
	if p.collectEndpoints {
		endpointsID := apiserver.EntityForEndpoints(castedObj.Namespace, castedObj.Name, "")
		log.Tracef("Invalidating configs on new/deleted service, endpoints entity: %s", endpointsID)
		delete(p.monitoredEndpoints, endpointsID)
	}
	p.upToDate = false
}

func (p *PrometheusServicesEndpointSlicesConfigProvider) invalidateIfChanged(old, obj interface{}) {
	// Cast the updated object, don't invalidate on casting error.
	// nil pointers are safely handled by the casting logic.
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %T", obj)
		return
	}

	// Cast the old object, invalidate on casting error
	castedOld, ok := old.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %T", old)
		p.setUpToDate(false)
		return
	}

	// Quick exit if resversion did not change
	if castedObj.ResourceVersion == castedOld.ResourceVersion {
		return
	}

	// Compare annotations
	if promAnnotationsDiffer(p.checks, castedObj.GetAnnotations(), castedOld.GetAnnotations()) {
		log.Trace("Invalidating configs on service change")
		p.setUpToDate(false)
		return
	}
}

func (p *PrometheusServicesEndpointSlicesConfigProvider) invalidateIfAddedEndpointSlices(_ interface{}) {
	// An endpointslice can be added after a service is created, in which case we need to re-run Collect
	p.setUpToDate(false)
}

func (p *PrometheusServicesEndpointSlicesConfigProvider) invalidateIfChangedEndpointSlices(old, obj interface{}) {
	// Cast the updated object, don't invalidate on casting error.
	// nil pointers are safely handled by the casting logic.
	castedObj, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an EndpointSlice type, got: %T", obj)
		return
	}

	// Cast the old object, invalidate on casting error
	castedOld, ok := old.(*discv1.EndpointSlice)
	if !ok {
		p.setUpToDate(false)
		return
	}

	// Quick exit if resversion did not change
	if castedObj.ResourceVersion == castedOld.ResourceVersion {
		return
	}

	// Get service name from labels
	serviceName := castedObj.Labels[apiserver.KubernetesServiceNameLabel]
	if serviceName == "" {
		return
	}

	// Make sure we invalidate a monitored endpoints object
	endpointsID := apiserver.EntityForEndpoints(castedObj.Namespace, serviceName, "")
	p.Lock()
	defer p.Unlock()
	if found := p.monitoredEndpoints[endpointsID]; found {
		// Invalidate only when endpoints change
		p.upToDate = equality.Semantic.DeepEqual(castedObj.Endpoints, castedOld.Endpoints)
	}
}

// GetConfigErrors is not implemented for the PrometheusServicesEndpointSlicesConfigProvider
func (p *PrometheusServicesEndpointSlicesConfigProvider) GetConfigErrors() map[string]providerTypes.ErrorMsgSet {
	return make(map[string]providerTypes.ErrorMsgSet)
}
