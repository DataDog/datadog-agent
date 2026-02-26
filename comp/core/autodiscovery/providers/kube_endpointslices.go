// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	listersv1 "k8s.io/client-go/listers/core/v1"
	disclisters "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kubeEndpointSliceAnnotationPrefix       = "ad.datadoghq.com/endpoints."
	kubeEndpointSliceAnnotationPrefixLegacy = "service-discovery.datadoghq.com/endpoints."
	kubeEndpointSliceResolvePath            = "resolve"
	kubernetesServiceNameLabelProvider      = "kubernetes.io/service-name"
)

// kubeEndpointSlicesConfigProvider implements the ConfigProvider interface for the apiserver using EndpointSlices.
type kubeEndpointSlicesConfigProvider struct {
	sync.RWMutex
	serviceLister       listersv1.ServiceLister
	endpointSliceLister disclisters.EndpointSliceLister
	upToDate            bool
	monitoredServices   map[string]bool // Key: "namespace/serviceName"
	configErrors        map[string]types.ErrorMsgSet
	telemetryStore      *telemetry.Store
}

// configInfoSlices contains an endpoint slice check config template with its namespace and service name
type configInfoSlices struct {
	tpl         integration.Config
	namespace   string
	serviceName string
	resolveMode endpointResolveMode
}

// NewKubeEndpointSlicesConfigProvider returns a new ConfigProvider connected to apiserver using EndpointSlices.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
// Using GetAPIClient (no wait) as Client should already be initialized by Cluster Agent main entrypoint before
func NewKubeEndpointSlicesConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, telemetryStore *telemetry.Store) (types.ConfigProvider, error) {
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	servicesInformer := ac.InformerFactory.Core().V1().Services()
	if servicesInformer == nil {
		return nil, fmt.Errorf("cannot get service informer: %s", err)
	}

	p := &kubeEndpointSlicesConfigProvider{
		serviceLister:     servicesInformer.Lister(),
		monitoredServices: make(map[string]bool),
		configErrors:      make(map[string]types.ErrorMsgSet),
		telemetryStore:    telemetryStore,
	}

	if _, err := servicesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.invalidateOnServiceAdd,
		UpdateFunc: p.invalidateOnServiceUpdate,
		DeleteFunc: p.invalidateOnServiceDelete,
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to service informer: %s", err)
	}

	endpointSliceInformer := ac.InformerFactory.Discovery().V1().EndpointSlices()
	if endpointSliceInformer == nil {
		return nil, fmt.Errorf("cannot get endpointslice informer: %s", err)
	}

	p.endpointSliceLister = endpointSliceInformer.Lister()

	if _, err := endpointSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: p.invalidateOnEndpointSliceUpdate,
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to endpointslice informer: %s", err)
	}

	if pkgconfigsetup.Datadog().GetBool("cluster_checks.support_hybrid_ignore_ad_tags") {
		log.Warnf("The `cluster_checks.support_hybrid_ignore_ad_tags` flag is" +
			" deprecated and will be removed in a future version. Please replace " +
			"`ad.datadoghq.com/endpoints.ignore_autodiscovery_tags` in your service annotations" +
			"using adv2 for check specification and adv1 for `ignore_autodiscovery_tags`.")
	}

	return p, nil
}

// String returns a string representation of the kubeEndpointSlicesConfigProvider
func (k *kubeEndpointSlicesConfigProvider) String() string {
	return names.KubeEndpointSlices
}

// Collect retrieves services from the apiserver, builds Config objects and returns them
func (k *kubeEndpointSlicesConfigProvider) Collect(context.Context) ([]integration.Config, error) {
	services, err := k.serviceLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	k.setUpToDate(true)

	var generatedConfigs []integration.Config
	parsedConfigsInfo := k.parseServiceAnnotationsForEndpointSlices(services)
	for _, conf := range parsedConfigsInfo {
		// Fetch all EndpointSlices for this service
		slices, err := k.endpointSliceLister.EndpointSlices(conf.namespace).List(
			labels.Set{kubernetesServiceNameLabelProvider: conf.serviceName}.AsSelector(),
		)
		if err != nil {
			log.Errorf("Cannot get Kubernetes endpointslices: %s", err)
			continue
		}

		// Generate ONE config per service (not per endpoint IP)
		if len(slices) > 0 {
			config := generateServiceLevelConfig(conf.tpl, conf.namespace, conf.serviceName)
			generatedConfigs = append(generatedConfigs, config)
		}

		serviceKey := fmt.Sprintf("%s/%s", conf.namespace, conf.serviceName)
		k.Lock()
		k.monitoredServices[serviceKey] = true
		k.Unlock()
	}
	return generatedConfigs, nil
}

// IsUpToDate allows to cache configs as long as no changes are detected in the apiserver
func (k *kubeEndpointSlicesConfigProvider) IsUpToDate(context.Context) (bool, error) {
	return k.upToDate, nil
}

// GetConfigErrors returns a map of configuration errors for each Kubernetes service
func (k *kubeEndpointSlicesConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return k.configErrors
}

func (k *kubeEndpointSlicesConfigProvider) invalidateOnServiceAdd(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Received unexpected object: %T", obj)
		return
	}

	if hasEndpointSliceAnnotations(svc) {
		k.setUpToDate(false)
	}
}

func (k *kubeEndpointSlicesConfigProvider) invalidateOnServiceDelete(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Errorf("Received unexpected object: %T", obj)
			return
		}

		svc, ok = tombstone.Obj.(*v1.Service)
		if !ok {
			log.Errorf("Expected a *v1.Service in the tombstone, got: %T", tombstone.Obj)
			return
		}
	}

	serviceKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
	k.Lock()
	defer k.Unlock()

	if k.monitoredServices[serviceKey] {
		delete(k.monitoredServices, serviceKey)
		k.upToDate = false
	}
}

func (k *kubeEndpointSlicesConfigProvider) invalidateOnServiceUpdate(old, obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a *v1.Service type, got: %T", obj)
		return
	}
	oldSvc, ok := old.(*v1.Service)
	if !ok {
		log.Errorf("Expected a *v1.Service type, got: %T", old)
		k.setUpToDate(false)
		return
	}
	if svc.ResourceVersion == oldSvc.ResourceVersion {
		return
	}

	serviceKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
	hasAnnotationsNow := hasEndpointSliceAnnotations(svc)
	hadAnnotationsBefore := hasEndpointSliceAnnotations(oldSvc)

	k.Lock()
	defer k.Unlock()
	isMonitored := k.monitoredServices[serviceKey]

	// Only invalidate if the service has endpoint annotations (now or before) or is being monitored
	if !hasAnnotationsNow && !hadAnnotationsBefore && !isMonitored {
		return
	}

	if !hasAnnotationsNow {
		delete(k.monitoredServices, serviceKey)
	}

	if valuesDiffer(svc.Annotations, oldSvc.Annotations, kubeEndpointSliceAnnotationPrefix) {
		log.Trace("Invalidating configs on service endpoint annotations change")
		k.upToDate = false
		return
	}
}

func (k *kubeEndpointSlicesConfigProvider) invalidateOnEndpointSliceUpdate(old, obj interface{}) {
	// Cast the updated object, don't invalidate on casting error.
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an *discv1.EndpointSlice type, got: %T", obj)
		return
	}
	// Cast the old object, invalidate on casting error
	oldSlice, ok := old.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected a *discv1.EndpointSlice type, got: %T", old)
		k.setUpToDate(false)
		return
	}
	// Quick exit if resource version did not change
	if slice.ResourceVersion == oldSlice.ResourceVersion {
		return
	}

	serviceName := slice.Labels[kubernetesServiceNameLabelProvider]
	if serviceName == "" {
		return
	}

	// Make sure we invalidate a monitored service
	serviceKey := fmt.Sprintf("%s/%s", slice.Namespace, serviceName)
	k.Lock()
	defer k.Unlock()
	if found := k.monitoredServices[serviceKey]; found {
		k.upToDate = equality.Semantic.DeepEqual(slice.Endpoints, oldSlice.Endpoints)
	}
}

// setUpToDate is a thread-safe method to update the upToDate value
func (k *kubeEndpointSlicesConfigProvider) setUpToDate(v bool) {
	k.Lock()
	defer k.Unlock()
	k.upToDate = v
}

func (k *kubeEndpointSlicesConfigProvider) parseServiceAnnotationsForEndpointSlices(services []*v1.Service) []configInfoSlices {
	var configsInfo []configInfoSlices

	setServiceKeys := map[string]struct{}{}

	for _, svc := range services {
		if svc == nil || svc.ObjectMeta.UID == "" {
			log.Debug("Ignoring a nil service")
			continue
		}

		serviceKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		setServiceKeys[serviceKey] = struct{}{}

		endptConf, errors := utils.ExtractTemplatesFromAnnotations(serviceKey, svc.GetAnnotations(), kubeEndpointID)
		for _, err := range errors {
			log.Errorf("Cannot parse endpoint template for service %s: %s", serviceKey, err)
		}

		if len(errors) > 0 {
			errMsgSet := make(types.ErrorMsgSet)
			for _, err := range errors {
				errMsgSet[err.Error()] = struct{}{}
			}
			k.configErrors[serviceKey] = errMsgSet
		} else {
			delete(k.configErrors, serviceKey)
		}

		var resolveMode endpointResolveMode
		if value, found := svc.Annotations[kubeEndpointSliceAnnotationPrefix+kubeEndpointSliceResolvePath]; found {
			resolveMode = endpointResolveMode(value)
		}

		ignoreAdForHybridScenariosTags := ignoreADTagsFromAnnotations(svc.GetAnnotations(), kubeEndpointSliceAnnotationPrefix)
		for i := range endptConf {
			endptConf[i].Source = "kube_endpoints:" + apiserver.EntityForEndpoints(svc.Namespace, svc.Name, "")
			if pkgconfigsetup.Datadog().GetBool("cluster_checks.support_hybrid_ignore_ad_tags") {
				endptConf[i].IgnoreAutodiscoveryTags = endptConf[i].IgnoreAutodiscoveryTags || ignoreAdForHybridScenariosTags
			}
			configsInfo = append(configsInfo, configInfoSlices{
				tpl:         endptConf[i],
				namespace:   svc.Namespace,
				serviceName: svc.Name,
				resolveMode: resolveMode,
			})
		}
	}

	k.cleanErrorsOfDeletedServices(setServiceKeys)

	if k.telemetryStore != nil {
		k.telemetryStore.Errors.Set(float64(len(k.configErrors)), names.KubeEndpoints)
	}

	return configsInfo
}

// hasEndpointSliceAnnotations checks if a service has any endpoint-related annotations
func hasEndpointSliceAnnotations(svc *v1.Service) bool {
	if svc == nil {
		return false
	}

	for key := range svc.Annotations {
		if strings.HasPrefix(key, kubeEndpointSliceAnnotationPrefix) || strings.HasPrefix(key, kubeEndpointSliceAnnotationPrefixLegacy) {
			return true
		}
	}

	return false
}

// generateServiceLevelConfig creates ONE config template for a service that matches all its endpoints
func generateServiceLevelConfig(tpl integration.Config, namespace, serviceName string) integration.Config {
	// Use service-level ADIdentifier that matches all endpoint services for this service
	serviceID := fmt.Sprintf("kube_endpoint://%s/%s", namespace, serviceName)

	newConfig := integration.Config{
		Name:                    tpl.Name,
		Instances:               tpl.Instances,
		InitConfig:              tpl.InitConfig,
		MetricConfig:            tpl.MetricConfig,
		LogsConfig:              tpl.LogsConfig,
		ADIdentifiers:           []string{serviceID},
		ClusterCheck:            true,
		Provider:                tpl.Provider,
		Source:                  tpl.Source,
		IgnoreAutodiscoveryTags: tpl.IgnoreAutodiscoveryTags,
	}

	return newConfig
}

func (k *kubeEndpointSlicesConfigProvider) cleanErrorsOfDeletedServices(setCurrentServiceKeys map[string]struct{}) {
	setServiceKeysWithErrors := map[string]struct{}{}
	for serviceKey := range k.configErrors {
		setServiceKeysWithErrors[serviceKey] = struct{}{}
	}

	for serviceKey := range setServiceKeysWithErrors {
		if _, exists := setCurrentServiceKeys[serviceKey]; !exists {
			delete(k.configErrors, serviceKey)
		}
	}
}
