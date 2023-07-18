// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func init() {
	RegisterProvider(names.KubeEndpointsFileRegisterName, NewKubeEndpointsFileConfigProvider)
}

type store struct {
	sync.RWMutex
	epConfigs map[string]*epConfig
}

func newStore() *store {
	return &store{epConfigs: make(map[string]*epConfig)}
}

type epConfig struct {
	templates     []integration.Config
	ep            *v1.Endpoints
	shouldCollect bool
}

func newEpConfig() *epConfig {
	return &epConfig{
		templates:     []integration.Config{},
		shouldCollect: false,
	}
}

// KubeEndpointsFileConfigProvider generates endpoints checks from check configurations defined in files.
type KubeEndpointsFileConfigProvider struct {
	sync.RWMutex
	epLister listersv1.EndpointsLister
	upToDate bool
	store    *store
}

// NewKubeEndpointsFileConfigProvider returns a new KubeEndpointsFileConfigProvider
func NewKubeEndpointsFileConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	templates, _, err := ReadConfigFiles(WithAdvancedADOnly)
	if err != nil {
		return nil, err
	}

	provider := &KubeEndpointsFileConfigProvider{}
	provider.buildConfigStore(templates)
	if provider.store.isEmpty() {
		provider.setUpToDate(true)
		return provider, nil
	}

	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	epInformer := ac.InformerFactory.Core().V1().Endpoints()
	if epInformer == nil {
		return nil, fmt.Errorf("cannot get endpoint informer: %s", err)
	}

	provider.epLister = epInformer.Lister()
	epInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    provider.addHandler,
		UpdateFunc: provider.updateHandler,
		DeleteFunc: provider.deleteHandler,
	})

	return provider, nil
}

// Collect returns the check configurations defined in Yaml files.
// Only configs with advanced AD identifiers targeting kubernetes endpoints are handled by this collector.
func (p *KubeEndpointsFileConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	p.setUpToDate(true)

	return p.store.generateConfigs(), nil
}

// IsUpToDate returns whether the config provider needs to be polled.
func (p *KubeEndpointsFileConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	p.RLock()
	defer p.RUnlock()

	return p.upToDate, nil
}

// String returns a string representation of the KubeEndpointsFileConfigProvider.
func (p *KubeEndpointsFileConfigProvider) String() string {
	return names.KubeEndpointsFile
}

// GetConfigErrors is not implemented for the KubeEndpointsFileConfigProvider.
func (p *KubeEndpointsFileConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}

func (p *KubeEndpointsFileConfigProvider) setUpToDate(v bool) {
	p.Lock()
	defer p.Unlock()

	p.upToDate = v
}

func (p *KubeEndpointsFileConfigProvider) addHandler(obj interface{}) {
	ep, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an Endpoints type, got: %T", obj)
		return
	}

	shouldUpdate := p.store.insertEp(ep)
	if shouldUpdate {
		p.setUpToDate(false)
	}
}

func (p *KubeEndpointsFileConfigProvider) updateHandler(old, new interface{}) {
	newEp, ok := new.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an Endpoints type, got: %T", new)
		return
	}

	if !p.store.shouldHandle(newEp) {
		return
	}

	oldEp, ok := old.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected a Endpoints type, got: %T", old)
		return
	}

	if !p.store.shouldHandle(oldEp) {
		return
	}

	if !equality.Semantic.DeepEqual(newEp.Subsets, oldEp.Subsets) {
		shouldUpdate := p.store.insertEp(newEp)
		if shouldUpdate {
			p.setUpToDate(false)
		}
	}
}

func (p *KubeEndpointsFileConfigProvider) deleteHandler(obj interface{}) {
	ep, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an Endpoints type, got: %T", obj)
		return
	}

	p.store.deleteEp(ep)
}

// buildConfigStore initializes the config templates store.
func (p *KubeEndpointsFileConfigProvider) buildConfigStore(templates []integration.Config) {
	p.store = newStore()
	for _, tpl := range templates {
		for _, advancedAD := range tpl.AdvancedADIdentifiers {
			if advancedAD.KubeEndpoints.IsEmpty() {
				continue
			}

			p.store.insertTemplate(epID(advancedAD.KubeEndpoints.Namespace, advancedAD.KubeEndpoints.Name), tpl)
		}
	}
}

// shouldHandle returns whether an endpoints object should be tracked.
func (s *store) shouldHandle(ep *v1.Endpoints) bool {
	s.RLock()
	defer s.RUnlock()

	_, found := s.epConfigs[epID(ep.Namespace, ep.Name)]
	return found
}

// insertTemplate caches config templates.
func (s *store) insertTemplate(id string, tpl integration.Config) {
	s.Lock()
	defer s.Unlock()

	_, found := s.epConfigs[id]
	if !found {
		s.epConfigs[id] = newEpConfig()
	}

	s.epConfigs[id].templates = append(s.epConfigs[id].templates, tpl)
}

// insertEp caches the provided endpoints object if it matches one of the tracked configs
// and prepares the config to be collected in the next Collect call.
// Returns false if the endpoints object is irrelevant and discarded.
func (s *store) insertEp(ep *v1.Endpoints) bool {
	s.Lock()
	defer s.Unlock()

	epConfig, found := s.epConfigs[epID(ep.Namespace, ep.Name)]
	if !found {
		return false
	}

	epConfig.ep = ep
	epConfig.shouldCollect = true

	return true
}

// deleteEp handles endpoint objects deletion.
// it marks the corresponfing config as not collectable.
func (s *store) deleteEp(ep *v1.Endpoints) {
	s.Lock()
	defer s.Unlock()

	epConfig, found := s.epConfigs[epID(ep.Namespace, ep.Name)]
	if !found {
		return
	}

	epConfig.ep = nil
	epConfig.shouldCollect = false
}

func (s *store) isEmpty() bool {
	s.RLock()
	defer s.RUnlock()

	return len(s.epConfigs) == 0
}

// generateConfigs transforms the cached config templates into collectable integration.Config objects
func (s *store) generateConfigs() []integration.Config {
	s.Lock()
	defer s.Unlock()

	configs := []integration.Config{}
	for _, epConfig := range s.epConfigs {
		if epConfig.shouldCollect {
			for _, tpl := range epConfig.templates {
				configs = append(configs, endpointChecksFromTemplate(tpl, epConfig.ep)...)
			}
		}
	}

	return configs
}

// endpointChecksFromTemplate resolves an integration.Config template based on the provided Endpoints object.
func endpointChecksFromTemplate(tpl integration.Config, ep *v1.Endpoints) []integration.Config {
	configs := []integration.Config{}
	if ep == nil {
		return configs
	}

	for i := range ep.Subsets {
		for j := range ep.Subsets[i].Addresses {
			entity := apiserver.EntityForEndpoints(ep.Namespace, ep.Name, ep.Subsets[i].Addresses[j].IP)
			config := &integration.Config{
				ServiceID:               entity,
				Name:                    tpl.Name,
				Instances:               tpl.Instances,
				InitConfig:              tpl.InitConfig,
				MetricConfig:            tpl.MetricConfig,
				LogsConfig:              tpl.LogsConfig,
				ADIdentifiers:           []string{entity},
				AdvancedADIdentifiers:   nil,
				ClusterCheck:            true,
				Provider:                names.KubeEndpointsFile,
				Source:                  tpl.Source,
				IgnoreAutodiscoveryTags: tpl.IgnoreAutodiscoveryTags,
			}

			utils.ResolveEndpointConfigAuto(config, ep.Subsets[i].Addresses[j])
			configs = append(configs, *config)
		}
	}

	return configs
}

func epID(ns, name string) string {
	return ns + "/" + name
}
