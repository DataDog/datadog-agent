// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver
// +build clusterchecks,kubeapiserver

package providers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	infov1 "k8s.io/client-go/informers/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// ServiceAPI abstracts the dependency on the Kubernetes API (useful for testing)
type ServiceAPI interface {
	// List lists all Services
	ListServices() ([]*v1.Service, error)
	// GetEndpoints gets Endpoints by namespace and name
	GetEndpoints(namespace, name string) (*v1.Endpoints, error)
}

type svcAPI struct {
	serviceLister   listersv1.ServiceLister
	endpointsLister listersv1.EndpointsLister
}

func (api *svcAPI) ListServices() ([]*v1.Service, error) {
	return api.serviceLister.List(labels.Everything())
}

func (api *svcAPI) GetEndpoints(namespace, name string) (*v1.Endpoints, error) {
	return api.endpointsLister.Endpoints(namespace).Get(name)
}

// PrometheusServicesConfigProvider implements the ConfigProvider interface for prometheus services
type PrometheusServicesConfigProvider struct {
	sync.RWMutex

	api      ServiceAPI
	upToDate bool

	collectEndpoints   bool
	monitoredEndpoints map[string]bool

	checks []*types.PrometheusCheck
}

// NewPrometheusServicesConfigProvider returns a new Prometheus ConfigProvider connected to kube apiserver
func NewPrometheusServicesConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	// Using GetAPIClient (no wait) as Client should already be initialized by Cluster Agent main entrypoint before
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	servicesInformer := ac.InformerFactory.Core().V1().Services()
	if servicesInformer == nil {
		return nil, errors.New("cannot get services informer")
	}

	var endpointsInformer infov1.EndpointsInformer
	var endpointsLister listersv1.EndpointsLister

	collectEndpoints := config.Datadog.GetBool("prometheus_scrape.service_endpoints")
	if collectEndpoints {
		endpointsInformer := ac.InformerFactory.Core().V1().Endpoints()
		if endpointsInformer == nil {
			return nil, errors.New("cannot get endpoints informer")
		}
		endpointsLister = endpointsInformer.Lister()
	}

	api := &svcAPI{
		serviceLister:   servicesInformer.Lister(),
		endpointsLister: endpointsLister,
	}

	checks, err := getPrometheusConfigs()
	if err != nil {
		return nil, err
	}

	p := newPromServicesProvider(checks, api, collectEndpoints)

	servicesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.invalidate,
		UpdateFunc: p.invalidateIfChanged,
		DeleteFunc: p.invalidate,
	})

	if endpointsInformer != nil {
		endpointsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: p.invalidateIfChangedEndpoints,
		})
	}
	return p, nil
}

func newPromServicesProvider(checks []*types.PrometheusCheck, api ServiceAPI, collectEndpoints bool) *PrometheusServicesConfigProvider {
	return &PrometheusServicesConfigProvider{
		checks:             checks,
		api:                api,
		collectEndpoints:   collectEndpoints,
		monitoredEndpoints: make(map[string]bool),
	}
}

// String returns a string representation of the PrometheusServicesConfigProvider
func (p *PrometheusServicesConfigProvider) String() string {
	return names.PrometheusServices
}

// Collect retrieves services from the apiserver, builds Config objects and returns them
func (p *PrometheusServicesConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	services, err := p.api.ListServices()
	if err != nil {
		return nil, err
	}

	var configs []integration.Config
	for _, svc := range services {
		for _, check := range p.checks {
			if !p.collectEndpoints {
				// Only generates Service checks is Endpoints checks are not active
				serviceConfigs := utils.ConfigsForService(check, svc)

				if len(serviceConfigs) != 0 {
					configs = append(configs, serviceConfigs...)
				}
			} else {
				ep, err := p.api.GetEndpoints(svc.GetNamespace(), svc.GetName())
				if err != nil {
					return nil, err
				}

				endpointConfigs := utils.ConfigsForServiceEndpoints(check, svc, ep)

				if len(endpointConfigs) == 0 {
					continue
				}

				configs = append(configs, endpointConfigs...)

				endpointsID := apiserver.EntityForEndpoints(ep.GetNamespace(), ep.GetName(), "")
				p.Lock()
				p.monitoredEndpoints[endpointsID] = true
				p.Unlock()
			}
		}
	}

	p.setUpToDate(true)
	return configs, nil
}

// setUpToDate is a thread-safe method to update the upToDate value
func (p *PrometheusServicesConfigProvider) setUpToDate(v bool) {
	p.Lock()
	defer p.Unlock()
	p.upToDate = v
}

// IsUpToDate allows to cache configs as long as no changes are detected in the apiserver
func (p *PrometheusServicesConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	p.RLock()
	defer p.RUnlock()
	return p.upToDate, nil
}

func (p *PrometheusServicesConfigProvider) invalidate(obj interface{}) {
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

func (p *PrometheusServicesConfigProvider) invalidateIfChanged(old, obj interface{}) {
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
	if p.promAnnotationsDiffer(castedObj.GetAnnotations(), castedOld.GetAnnotations()) {
		log.Trace("Invalidating configs on service change")
		p.setUpToDate(false)
		return
	}
}

func (p *PrometheusServicesConfigProvider) invalidateIfChangedEndpoints(old, obj interface{}) {
	// Cast the updated object, don't invalidate on casting error.
	// nil pointers are safely handled by the casting logic.
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected a Endpoints type, got: %T", obj)
		return
	}

	// Cast the old object, invalidate on casting error
	castedOld, ok := old.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected a Endpoints type, got: %T", old)
		p.setUpToDate(false)
		return
	}

	// Quick exit if resversion did not change
	if castedObj.ResourceVersion == castedOld.ResourceVersion {
		return
	}

	// Make sure we invalidate a monitored endpoints object
	endpointsID := apiserver.EntityForEndpoints(castedObj.Namespace, castedObj.Name, "")
	p.Lock()
	defer p.Unlock()
	if found := p.monitoredEndpoints[endpointsID]; found {
		// Invalidate only when subsets change
		p.upToDate = equality.Semantic.DeepEqual(castedObj.Subsets, castedOld.Subsets)
	}
}

// promAnnotationsDiffer returns whether a service update corresponds to a config invalidation
func (p *PrometheusServicesConfigProvider) promAnnotationsDiffer(first, second map[string]string) bool {
	for _, annotation := range types.PrometheusStandardAnnotations {
		if first[annotation] != second[annotation] {
			return true
		}
	}

	for _, check := range p.checks {
		for k := range check.AD.GetIncludeAnnotations() {
			if first[k] != second[k] {
				return true
			}
		}
		for k := range check.AD.GetExcludeAnnotations() {
			if first[k] != second[k] {
				return true
			}
		}
	}

	return false
}

func init() {
	RegisterProvider(names.PrometheusServicesRegisterName, NewPrometheusServicesConfigProvider)
}

// GetConfigErrors is not implemented for the PrometheusServicesConfigProvider
func (p *PrometheusServicesConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
