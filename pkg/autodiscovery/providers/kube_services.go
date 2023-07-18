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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kubeServiceID               = "service"
	kubeServiceAnnotationPrefix = "ad.datadoghq.com/service."
)

// KubeServiceConfigProvider implements the ConfigProvider interface for the apiserver.
type KubeServiceConfigProvider struct {
	lister   listersv1.ServiceLister
	upToDate bool
}

// NewKubeServiceConfigProvider returns a new ConfigProvider connected to apiserver.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewKubeServiceConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	// Using GetAPIClient() (no retry)
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	servicesInformer := ac.InformerFactory.Core().V1().Services()
	if servicesInformer == nil {
		return nil, fmt.Errorf("cannot get service informer: %s", err)
	}

	p := &KubeServiceConfigProvider{
		lister: servicesInformer.Lister(),
	}

	servicesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.invalidate,
		UpdateFunc: p.invalidateIfChanged,
		DeleteFunc: p.invalidate,
	})

	return p, nil
}

// String returns a string representation of the KubeServiceConfigProvider
func (k *KubeServiceConfigProvider) String() string {
	return names.KubeServices
}

// Collect retrieves services from the apiserver, builds Config objects and returns them
func (k *KubeServiceConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	services, err := k.lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	k.upToDate = true

	return parseServiceAnnotations(services)
}

// IsUpToDate allows to cache configs as long as no changes are detected in the apiserver
func (k *KubeServiceConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	return k.upToDate, nil
}

func (k *KubeServiceConfigProvider) invalidate(obj interface{}) {
	if obj != nil {
		log.Trace("Invalidating configs on new/deleted service")
		k.upToDate = false
	}
}

func (k *KubeServiceConfigProvider) invalidateIfChanged(old, obj interface{}) {
	// Cast the updated object, don't invalidate on casting error.
	// nil pointers are safely handled by the casting logic.
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a *v1.Service type, got: %T", obj)
		return
	}
	// Cast the old object, invalidate on casting error
	castedOld, ok := old.(*v1.Service)
	if !ok {
		log.Errorf("Expected a *v1.Service type, got: %T", old)
		k.upToDate = false
		return
	}
	// Quick exit if resversion did not change
	if castedObj.ResourceVersion == castedOld.ResourceVersion {
		return
	}
	// Compare annotations
	if valuesDiffer(castedObj.Annotations, castedOld.Annotations, kubeServiceAnnotationPrefix) {
		log.Trace("Invalidating configs on service change")
		k.upToDate = false
		return
	}
}

// valuesDiffer returns true if the annotations matching the
// given prefix are different between map first and second.
// It also counts the annotation count to catch deletions.
func valuesDiffer(first, second map[string]string, prefix string) bool {
	var matchingInFirst int
	for name, value := range first {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if second[name] != value {
			return true
		}
		matchingInFirst++
	}

	var matchingInSecond int
	for name := range second {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		matchingInSecond++
	}

	return matchingInFirst != matchingInSecond
}

func parseServiceAnnotations(services []*v1.Service) ([]integration.Config, error) {
	var configs []integration.Config

	for _, svc := range services {
		if svc == nil || svc.ObjectMeta.UID == "" {
			log.Debug("Ignoring a nil service")
			continue
		}

		serviceID := apiserver.EntityForService(svc)
		svcConf, errors := utils.ExtractTemplatesFromPodAnnotations(serviceID, svc.Annotations, kubeServiceID)
		for _, err := range errors {
			log.Errorf("Cannot parse service template for service %s/%s: %s", svc.Namespace, svc.Name, err)
		}

		ignoreADTags := ignoreADTagsFromAnnotations(svc.GetAnnotations(), kubeServiceAnnotationPrefix)

		// All configurations are cluster checks
		for i := range svcConf {
			svcConf[i].ClusterCheck = true
			svcConf[i].Source = "kube_services:" + serviceID
			svcConf[i].IgnoreAutodiscoveryTags = ignoreADTags
		}

		configs = append(configs, svcConf...)
	}

	return configs, nil
}

func init() {
	RegisterProvider(names.KubeServicesRegisterName, NewKubeServiceConfigProvider)
}

// GetConfigErrors is not implemented for the KubeServiceConfigProvider
func (k *KubeServiceConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
