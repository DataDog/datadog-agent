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

	discv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	discv1listers "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type serviceTracker interface {
	NotifyOnChange(fn func(namespace, name string))
	HasService(namespace string, name string) bool
	AllTemplatesByService() map[string][]integration.Config
}

// KubeEndpointSlicesCRConfigProvider generates endpoint check configs from
// DatadogInstrumentation CRs targeting Services. It reads check templates from
// a ServiceCheckTemplateStore (populated by the DDI handler), watches
// EndpointSlice informer events, and produces per-endpoint integration.Config
type KubeEndpointSlicesCRConfigProvider struct {
	mu             sync.RWMutex
	upToDate       bool
	serviceTracker serviceTracker
	epSliceLister  discv1listers.EndpointSliceLister
}

// NewKubeEndpointSlicesCRConfigProvider returns a new KubeEndpointSlicesCRConfigProvider.
func NewKubeEndpointSlicesCRConfigProvider(serviceTracker serviceTracker) (types.ConfigProvider, error) {
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver for endpoint slices CR provider: %w", err)
	}

	epSliceInformer := ac.InformerFactory.Discovery().V1().EndpointSlices()

	p := &KubeEndpointSlicesCRConfigProvider{
		serviceTracker: serviceTracker,
		epSliceLister:  epSliceInformer.Lister(),
	}

	// Mark dirty when templates change (DDI CR created/updated/deleted).
	serviceTracker.NotifyOnChange(func(string, string) {
		p.setUpToDate(false)
	})

	// Mark dirty when a tracked service's EndpointSlice changes so Collect re-queries the lister.
	if _, err := epSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    p.addHandler,
		UpdateFunc: p.updateHandler,
		DeleteFunc: p.deleteHandler,
	}); err != nil {
		return nil, fmt.Errorf("cannot register EndpointSlice event handler: %w", err)
	}

	return p, nil
}

// Collect generates per-endpoint integration.Config entries by combining
// check templates from the template store with EndpointSlices from the lister.
func (p *KubeEndpointSlicesCRConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	p.setUpToDate(true)
	return p.generateConfigs(), nil
}

// IsUpToDate returns whether the provider needs to be polled.
func (p *KubeEndpointSlicesCRConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.upToDate, nil
}

// String returns the provider name.
func (p *KubeEndpointSlicesCRConfigProvider) String() string {
	return names.KubeEndpointSlicesCR
}

// GetConfigErrors returns a map of configuration errors (none for this provider).
func (p *KubeEndpointSlicesCRConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return nil
}

func (p *KubeEndpointSlicesCRConfigProvider) setUpToDate(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.upToDate = v
}

func (p *KubeEndpointSlicesCRConfigProvider) isTracked(slice *discv1.EndpointSlice) bool {
	svcName := slice.Labels[kubernetesServiceNameLabelProvider]
	return svcName != "" && p.serviceTracker.HasService(slice.Namespace, svcName)
}

func (p *KubeEndpointSlicesCRConfigProvider) addHandler(obj interface{}) {
	if slice, ok := obj.(*discv1.EndpointSlice); ok && p.isTracked(slice) {
		p.setUpToDate(false)
	}
}

func (p *KubeEndpointSlicesCRConfigProvider) updateHandler(oldObj, newObj interface{}) {
	oldSlice, oldOk := oldObj.(*discv1.EndpointSlice)
	newSlice, newOk := newObj.(*discv1.EndpointSlice)
	if !oldOk || !newOk {
		return
	}
	if p.isTracked(newSlice) && !equality.Semantic.DeepEqual(oldSlice.Endpoints, newSlice.Endpoints) {
		p.setUpToDate(false)
	}
}

func (p *KubeEndpointSlicesCRConfigProvider) deleteHandler(obj interface{}) {
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		slice, ok = tombstone.Obj.(*discv1.EndpointSlice)
		if !ok {
			return
		}
	}
	if p.isTracked(slice) {
		p.setUpToDate(false)
	}
}

// generateConfigs queries the lister for EndpointSlices matching each tracked
// service and combines them with the corresponding check templates.
func (p *KubeEndpointSlicesCRConfigProvider) generateConfigs() []integration.Config {
	templatesByService := p.serviceTracker.AllTemplatesByService()
	if len(templatesByService) == 0 {
		return nil
	}

	var configs []integration.Config
	for svcKey, templates := range templatesByService {
		ns, name, _ := strings.Cut(svcKey, "/")
		selector := labels.SelectorFromSet(labels.Set{kubernetesServiceNameLabelProvider: name})
		slices, err := p.epSliceLister.EndpointSlices(ns).List(selector)
		if err != nil {
			log.Warnf("failed to list EndpointSlices for %s: %v", svcKey, err)
			continue
		}
		if len(slices) == 0 {
			log.Infof("service %s has %d template(s) but 0 EndpointSlices", svcKey, len(templates))
			continue
		}
		for _, slice := range slices {
			for _, tpl := range templates {
				configs = append(configs, endpointSliceChecksFromTemplate(tpl, slice, kubeEndpointResolveAuto)...)
			}
		}
	}

	for i := range configs {
		configs[i].Provider = names.KubeEndpointSlicesCR
	}

	return configs
}
