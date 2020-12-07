// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package providers

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// PrometheusServicesConfigProvider implements the ConfigProvider interface for prometheus services
type PrometheusServicesConfigProvider struct {
	lister   listersv1.ServiceLister
	upToDate bool
	PrometheusConfigProvider
}

// NewPrometheusServicesConfigProvider returns a new Prometheus ConfigProvider connected to kube apiserver
func NewPrometheusServicesConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	servicesInformer := ac.InformerFactory.Core().V1().Services()
	if servicesInformer == nil {
		return nil, errors.New("cannot get service informer")
	}

	p := &PrometheusServicesConfigProvider{
		lister: servicesInformer.Lister(),
	}

	servicesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.invalidate,
		UpdateFunc: p.invalidateIfChanged,
		DeleteFunc: p.invalidate,
	})

	err = p.setupConfigs()
	return p, err
}

// String returns a string representation of the PrometheusServicesConfigProvider
func (p *PrometheusServicesConfigProvider) String() string {
	return names.PrometheusServices
}

// Collect retrieves services from the apiserver, builds Config objects and returns them
func (p *PrometheusServicesConfigProvider) Collect() ([]integration.Config, error) {
	services, err := p.lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	p.upToDate = true
	return p.parseServices(services), nil
}

// IsUpToDate allows to cache configs as long as no changes are detected in the apiserver
func (p *PrometheusServicesConfigProvider) IsUpToDate() (bool, error) {
	return p.upToDate, nil
}

// parseServices returns a list of configurations based on the service annotations
func (p *PrometheusServicesConfigProvider) parseServices(services []*v1.Service) []integration.Config {
	var configs []integration.Config
	for _, svc := range services {
		for _, check := range p.checks {
			configs = append(configs, check.ConfigsForService(svc)...)
		}
	}
	return configs
}

func (p *PrometheusServicesConfigProvider) invalidate(obj interface{}) {
	if obj != nil {
		log.Trace("Invalidating configs on new/deleted service")
		p.upToDate = false
	}
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
		p.upToDate = false
		return
	}

	// Quick exit if resversion did not change
	if castedObj.ResourceVersion == castedOld.ResourceVersion {
		return
	}

	// Compare annotations
	if p.promAnnotationsDiffer(castedObj.GetAnnotations(), castedOld.GetAnnotations()) {
		log.Trace("Invalidating configs on service change")
		p.upToDate = false
		return
	}
}

// promAnnotationsDiffer returns whether a service update corresponds to a config invalidation
func (p *PrometheusServicesConfigProvider) promAnnotationsDiffer(first, second map[string]string) bool {
	for _, annotation := range common.PrometheusStandardAnnotations {
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
	RegisterProvider("prometheus_services", NewPrometheusServicesConfigProvider)
}
