// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package providers

import (
	"context"
	"fmt"
	"sync"

	discv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type serviceTemplateStore interface {
	SetOnChange(func())
	HasService(string, string) bool
	AllTemplatesByService() map[string][]integration.Config
}

// KubeEndpointSlicesCRConfigProvider generates endpoint check configs from
// DatadogInstrumentation CRs targeting Services. It reads check templates from
// a ServiceCheckTemplateStore (populated by the DDI handler), watches
// EndpointSlice informer events, and produces per-endpoint integration.Config
// entries that flow through the cluster check dispatcher pipeline.
type KubeEndpointSlicesCRConfigProvider struct {
	mu            sync.RWMutex
	upToDate      bool
	templateStore serviceTemplateStore
	// slicesByService maps service "namespace/name" to a map of EndpointSlice UID → slice.
	slicesByService map[string]map[string]*discv1.EndpointSlice
}

// NewKubeEndpointSlicesCRConfigProvider returns a new KubeEndpointSlicesCRConfigProvider.
// It registers EndpointSlice informer event handlers and hooks into the template
// store's onChange callback to mark the provider as dirty when templates change.
func NewKubeEndpointSlicesCRConfigProvider(templateStore serviceTemplateStore) (*KubeEndpointSlicesCRConfigProvider, error) {
	provider := &KubeEndpointSlicesCRConfigProvider{
		templateStore:   templateStore,
		slicesByService: make(map[string]map[string]*discv1.EndpointSlice),
	}

	templateStore.SetOnChange(func() {
		provider.setUpToDate(false)
	})

	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver for endpoint slices CR provider: %w", err)
	}

	epSliceInformer := ac.InformerFactory.Discovery().V1().EndpointSlices()
	if _, err := epSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    provider.addHandler,
		UpdateFunc: provider.updateHandler,
		DeleteFunc: provider.deleteHandler,
	}); err != nil {
		return nil, fmt.Errorf("cannot register EndpointSlice event handler: %w", err)
	}

	return provider, nil
}

// Collect generates per-endpoint integration.Config entries by combining
// check templates from the ServiceCheckTemplateStore with cached EndpointSlices.
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

func (p *KubeEndpointSlicesCRConfigProvider) addHandler(obj interface{}) {
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected *discv1.EndpointSlice, got: %T", obj)
		return
	}
	if p.insertSlice(slice) {
		p.setUpToDate(false)
	}
}

func (p *KubeEndpointSlicesCRConfigProvider) updateHandler(oldObj, newObj interface{}) {
	newSlice, ok := newObj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected *discv1.EndpointSlice, got: %T", newObj)
		return
	}
	oldSlice, ok := oldObj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected *discv1.EndpointSlice, got: %T", oldObj)
		return
	}

	if !equality.Semantic.DeepEqual(newSlice.Endpoints, oldSlice.Endpoints) {
		if p.replaceSlice(oldSlice, newSlice) {
			p.setUpToDate(false)
		}
	}
}

func (p *KubeEndpointSlicesCRConfigProvider) deleteHandler(obj interface{}) {
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Errorf("Expected *discv1.EndpointSlice or DeletedFinalStateUnknown, got: %T", obj)
			return
		}
		slice, ok = tombstone.Obj.(*discv1.EndpointSlice)
		if !ok {
			log.Errorf("Expected *discv1.EndpointSlice in tombstone, got: %T", tombstone.Obj)
			return
		}
	}
	if p.deleteSlice(slice) {
		p.setUpToDate(false)
	}
}

// serviceKeyForSlice returns the composite key, namespace, and service name for an EndpointSlice.
func serviceKeyForSlice(slice *discv1.EndpointSlice) (key, ns, name string, ok bool) {
	serviceName := slice.Labels[kubernetesServiceNameLabelProvider]
	if serviceName == "" {
		return "", "", "", false
	}
	return slice.Namespace + "/" + serviceName, slice.Namespace, serviceName, true
}

// insertSlice caches an EndpointSlice if its parent service is tracked by the template store.
func (p *KubeEndpointSlicesCRConfigProvider) insertSlice(slice *discv1.EndpointSlice) bool {
	svcKey, ns, name, ok := serviceKeyForSlice(slice)
	if !ok {
		return false
	}

	if !p.templateStore.HasService(ns, name) {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.slicesByService[svcKey] == nil {
		p.slicesByService[svcKey] = make(map[string]*discv1.EndpointSlice)
	}
	p.slicesByService[svcKey][string(slice.UID)] = slice
	return true
}

// replaceSlice atomically removes the old slice and inserts the new one under a single lock.
func (p *KubeEndpointSlicesCRConfigProvider) replaceSlice(oldSlice, newSlice *discv1.EndpointSlice) bool {
	oldKey, _, _, oldOk := serviceKeyForSlice(oldSlice)
	newKey, newNs, newName, newOk := serviceKeyForSlice(newSlice)

	p.mu.Lock()
	defer p.mu.Unlock()

	changed := false

	// Remove old
	if oldOk {
		if slices, found := p.slicesByService[oldKey]; found {
			if _, exists := slices[string(oldSlice.UID)]; exists {
				delete(slices, string(oldSlice.UID))
				if len(slices) == 0 {
					delete(p.slicesByService, oldKey)
				}
				changed = true
			}
		}
	}

	// Insert new (only if service is tracked)
	if newOk && p.templateStore.HasService(newNs, newName) {
		if p.slicesByService[newKey] == nil {
			p.slicesByService[newKey] = make(map[string]*discv1.EndpointSlice)
		}
		p.slicesByService[newKey][string(newSlice.UID)] = newSlice
		changed = true
	}

	return changed
}

// deleteSlice removes a cached EndpointSlice.
func (p *KubeEndpointSlicesCRConfigProvider) deleteSlice(slice *discv1.EndpointSlice) bool {
	svcKey, _, _, ok := serviceKeyForSlice(slice)
	if !ok {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	slices, found := p.slicesByService[svcKey]
	if !found {
		return false
	}
	uid := string(slice.UID)
	if _, exists := slices[uid]; !exists {
		return false
	}
	delete(slices, uid)
	if len(slices) == 0 {
		delete(p.slicesByService, svcKey)
	}
	return true
}

// generateConfigs combines templates from the store with cached EndpointSlices
// to produce per-endpoint integration.Config entries.
//
// To avoid deadlock, we snapshot the slice data under p.mu, then release it
// before calling into the template store (which acquires its own lock).
func (p *KubeEndpointSlicesCRConfigProvider) generateConfigs() []integration.Config {
	// Fetch all templates in a single lock acquisition on the template store.
	templatesByService := p.templateStore.AllTemplatesByService()

	// Snapshot slice data under p.mu.
	p.mu.RLock()
	type svcSlices struct {
		templates []integration.Config
		slices    []*discv1.EndpointSlice
	}
	var work []svcSlices
	for svcKey, templates := range templatesByService {
		sliceMap := p.slicesByService[svcKey]
		if len(sliceMap) == 0 {
			continue
		}
		ss := svcSlices{templates: templates}
		for _, s := range sliceMap {
			ss.slices = append(ss.slices, s)
		}
		work = append(work, ss)
	}
	p.mu.RUnlock()

	// Generate configs without holding any lock.
	var configs []integration.Config
	for _, ss := range work {
		for _, slice := range ss.slices {
			for _, tpl := range ss.templates {
				configs = append(configs, endpointSliceChecksFromTemplate(tpl, slice, kubeEndpointResolveAuto)...)
			}
		}
	}

	// The Provider field must identify this config provider so the AD dispatcher
	// routes these configs through the cluster check pipeline.
	for i := range configs {
		configs[i].Provider = names.KubeEndpointSlicesCR
	}

	return configs
}
