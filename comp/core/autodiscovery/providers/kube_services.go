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

	"go.uber.org/atomic"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kubeServiceID               = "service"
	kubeServiceAnnotationPrefix = "ad.datadoghq.com/service."
)

// KubeServiceConfigProvider implements the ConfigProvider interface for the apiserver.
type KubeServiceConfigProvider struct {
	lister         listersv1.ServiceLister
	upToDate       *atomic.Bool
	configErrors   map[string]types.ErrorMsgSet
	telemetryStore *telemetry.Store
}

// NewKubeServiceConfigProvider returns a new ConfigProvider connected to apiserver.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewKubeServiceConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, telemetryStore *telemetry.Store) (types.ConfigProvider, error) {
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
		lister:         servicesInformer.Lister(),
		configErrors:   make(map[string]types.ErrorMsgSet),
		telemetryStore: telemetryStore,
		upToDate:       atomic.NewBool(false),
	}

	if _, err := servicesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.invalidate,
		UpdateFunc: p.invalidateIfChanged,
		DeleteFunc: p.invalidate,
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to services informer: %s", err)
	}

	if pkgconfigsetup.Datadog().GetBool("cluster_checks.support_hybrid_ignore_ad_tags") {
		log.Warnf("The `cluster_checks.support_hybrid_ignore_ad_tags` flag is" +
			" deprecated and will be removed in a future version. Please replace " +
			"`ad.datadoghq.com/service.ignore_autodiscovery_tags` in your service annotations" +
			"using adv2 for check specification and adv1 for `ignore_autodiscovery_tags`.")
	}

	return p, nil
}

// String returns a string representation of the KubeServiceConfigProvider
func (k *KubeServiceConfigProvider) String() string {
	return names.KubeServices
}

// Collect retrieves services from the apiserver, builds Config objects and returns them
func (k *KubeServiceConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	services, err := k.lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	k.upToDate.Store(true)

	return k.parseServiceAnnotations(services, pkgconfigsetup.Datadog())
}

// IsUpToDate allows to cache configs as long as no changes are detected in the apiserver
func (k *KubeServiceConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	return k.upToDate.Load(), nil
}

func (k *KubeServiceConfigProvider) invalidate(obj interface{}) {
	if obj != nil {
		log.Trace("Invalidating configs on new/deleted service")
		k.upToDate.Store(false)
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
		k.upToDate.Store(false)
		return
	}
	// Quick exit if resversion did not change
	if castedObj.ResourceVersion == castedOld.ResourceVersion {
		return
	}
	// Compare annotations
	if valuesDiffer(castedObj.Annotations, castedOld.Annotations, kubeServiceAnnotationPrefix) {
		log.Trace("Invalidating configs on service change")
		k.upToDate.Store(false)
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

func (k *KubeServiceConfigProvider) parseServiceAnnotations(services []*v1.Service, ddConf model.Config) ([]integration.Config, error) {
	var configs []integration.Config

	setServiceIDs := map[string]struct{}{}

	for _, svc := range services {
		if svc == nil || svc.ObjectMeta.UID == "" {
			log.Debug("Ignoring a nil service")
			continue
		}

		serviceID := apiserver.EntityForService(svc)
		setServiceIDs[serviceID] = struct{}{}
		svcConf, errors := utils.ExtractTemplatesFromAnnotations(serviceID, svc.Annotations, kubeServiceID)
		if len(errors) > 0 {
			errMsgSet := make(types.ErrorMsgSet)
			for _, err := range errors {
				log.Errorf("Cannot parse service template for service %s/%s: %s", svc.Namespace, svc.Name, err)
				errMsgSet[err.Error()] = struct{}{}
			}
			k.configErrors[serviceID] = errMsgSet
		} else {
			delete(k.configErrors, serviceID)
		}

		ignoreAdForHybridScenariosTags := ignoreADTagsFromAnnotations(svc.GetAnnotations(), kubeServiceAnnotationPrefix)
		// All configurations are cluster checks
		for i := range svcConf {
			svcConf[i].ClusterCheck = true
			svcConf[i].Source = "kube_services:" + serviceID
			// TODO(CINT)(Agent 7.53+) Remove support for hybrid scenarios
			if ddConf.GetBool("cluster_checks.support_hybrid_ignore_ad_tags") {
				svcConf[i].IgnoreAutodiscoveryTags = svcConf[i].IgnoreAutodiscoveryTags || ignoreAdForHybridScenariosTags
			}
		}

		configs = append(configs, svcConf...)
	}

	k.cleanErrorsOfDeletedServices(setServiceIDs)

	if k.telemetryStore != nil {
		k.telemetryStore.Errors.Set(float64(len(k.configErrors)), names.KubeServices)
	}

	return configs, nil
}

func (k *KubeServiceConfigProvider) cleanErrorsOfDeletedServices(setCurrentServiceIDs map[string]struct{}) {
	setServiceIDsWithErrors := map[string]struct{}{}
	for serviceID := range k.configErrors {
		setServiceIDsWithErrors[serviceID] = struct{}{}
	}

	for serviceID := range setServiceIDsWithErrors {
		if _, exists := setCurrentServiceIDs[serviceID]; !exists {
			delete(k.configErrors, serviceID)
		}
	}
}

// GetConfigErrors returns a map of configuration errors for each Kubernetes service
func (k *KubeServiceConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return k.configErrors
}
