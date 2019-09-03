// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package providers

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kubeEndpointAnnotationPrefix = "ad.datadoghq.com/endpoints."
	kubePodKind                  = "Pod"
	KubePodPrefix                = "kubernetes_pod://"
)

// KubeEndpointsConfigProvider implements the ConfigProvider interface for the apiserver.
type KubeEndpointsConfigProvider struct {
	serviceLister   listersv1.ServiceLister
	endpointsLister listersv1.EndpointsLister
	upToDate        bool
}

// configInfo contains an endpoint check config template with its name and namespace
type configInfo struct {
	tpl       integration.Config
	namespace string
	name      string
}

// NewKubeEndpointsConfigProvider returns a new ConfigProvider connected to apiserver.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewKubeEndpointsConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}
	servicesInformer := ac.InformerFactory.Core().V1().Services()
	if servicesInformer == nil {
		return nil, fmt.Errorf("cannot get service informer: %s", err)
	}

	p := &KubeEndpointsConfigProvider{
		serviceLister: servicesInformer.Lister(),
	}

	servicesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.invalidate,
		UpdateFunc: p.invalidateIfChanged,
		DeleteFunc: p.invalidate,
	})

	endpointsInformer := ac.InformerFactory.Core().V1().Endpoints()
	if endpointsInformer == nil {
		return nil, fmt.Errorf("cannot get endpoint informer: %s", err)
	}
	p.endpointsLister = endpointsInformer.Lister()

	return p, nil
}

// String returns a string representation of the KubeEndpointsConfigProvider
func (k *KubeEndpointsConfigProvider) String() string {
	return KubeEndpoints
}

// Collect retrieves services from the apiserver, builds Config objects and returns them
func (k *KubeEndpointsConfigProvider) Collect() ([]integration.Config, error) {
	services, err := k.serviceLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	k.upToDate = true

	var generatedConfigs []integration.Config
	parsedConfigsInfo := parseServiceAnnotationsForEndpoints(services)
	for _, config := range parsedConfigsInfo {
		kep, err := k.endpointsLister.Endpoints(config.namespace).Get(config.name)
		if err != nil {
			log.Errorf("Cannot get Kubernetes endpoints: %s", err)
			continue
		}
		generatedConfigs = append(generatedConfigs, generateConfigs(config.tpl, kep)...)
	}
	return generatedConfigs, nil
}

// IsUpToDate allows to cache configs as long as no changes are detected in the apiserver
func (k *KubeEndpointsConfigProvider) IsUpToDate() (bool, error) {
	return k.upToDate, nil
}

func (k *KubeEndpointsConfigProvider) invalidate(obj interface{}) {
	if obj != nil {
		log.Trace("Invalidating configs on new/deleted service")
		k.upToDate = false
	}
}

func (k *KubeEndpointsConfigProvider) invalidateIfChanged(old, obj interface{}) {
	// Cast the updated object, don't invalidate on casting error.
	// nil pointers are safely handled by the casting logic.
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", obj)
		return
	}
	// Cast the old object, invalidate on casting error
	castedOld, ok := old.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", old)
		k.upToDate = false
		return
	}
	// Quick exit if resversion did not change
	if castedObj.ResourceVersion == castedOld.ResourceVersion {
		return
	}
	if valuesDiffer(castedObj.Annotations, castedOld.Annotations, kubeEndpointAnnotationPrefix) {
		log.Trace("Invalidating configs on service end annotations change")
		k.upToDate = false
		return
	}
}

func parseServiceAnnotationsForEndpoints(services []*v1.Service) []configInfo {
	var configsInfo []configInfo
	for _, svc := range services {
		if svc == nil || svc.ObjectMeta.UID == "" {
			log.Debug("Ignoring a nil service")
			continue
		}
		endptConf, errors := extractTemplatesFromMap(apiserver.EntityForEndpoints(svc.Namespace, svc.Name, ""), svc.Annotations, kubeEndpointAnnotationPrefix)
		for _, err := range errors {
			log.Errorf("Cannot parse endpoint template for service %s/%s: %s", svc.Namespace, svc.Name, err)
		}
		for i := range endptConf {
			configsInfo = append(configsInfo, configInfo{
				tpl:       endptConf[i],
				namespace: svc.Namespace,
				name:      svc.Name,
			})
		}
	}
	return configsInfo
}

// generateConfigs creates a config template for each Endpoints IP
func generateConfigs(tpl integration.Config, kep *v1.Endpoints) []integration.Config {
	if kep == nil {
		log.Warn("Nil Kubernetes Endpoints object, cannot generate config templates")
		return []integration.Config{tpl}
	}
	generatedConfigs := []integration.Config{}
	namespace := kep.Namespace
	name := kep.Name
	for i := range kep.Subsets {
		for j := range kep.Subsets[i].Addresses {
			// Set a new entity containing the endpoint's IP
			entity := apiserver.EntityForEndpoints(namespace, name, kep.Subsets[i].Addresses[j].IP)
			newConfig := integration.Config{
				Entity:        entity,
				Name:          tpl.Name,
				Instances:     tpl.Instances,
				InitConfig:    tpl.InitConfig,
				MetricConfig:  tpl.MetricConfig,
				LogsConfig:    tpl.LogsConfig,
				ADIdentifiers: []string{entity},
				ClusterCheck:  true,
				Provider:      tpl.Provider,
				Source:        tpl.Source,
			}
			if targetRef := kep.Subsets[i].Addresses[j].TargetRef; targetRef != nil {
				if targetRef.Kind == kubePodKind {
					// The endpoint is backed by a pod.
					// We add the pod uid as AD identifiers so the check can get the pod tags.
					podUID := string(targetRef.UID)
					newConfig.ADIdentifiers = append(newConfig.ADIdentifiers, getPodEntity(podUID))
					if nodeName := kep.Subsets[i].Addresses[j].NodeName; nodeName != nil {
						// Set the node name to schedule the endpoint check on the correct node.
						// This field needs to be set only when the endpoint is backed by a pod.
						newConfig.NodeName = *nodeName
					}
				}
			}
			generatedConfigs = append(generatedConfigs, newConfig)
		}
	}
	return generatedConfigs
}

// getPodEntity returns pod entity
func getPodEntity(podUID string) string {
	return KubePodPrefix + podUID
}

func init() {
	RegisterProvider(KubeEndpointsProviderName, NewKubeEndpointsConfigProvider)
}
