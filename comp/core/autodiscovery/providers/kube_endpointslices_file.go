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

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/apimachinery/pkg/api/equality"

	discv1 "k8s.io/api/discovery/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	celEndpointSliceID = "cel://endpoint-slice"
)

type endpointSliceStore struct {
	sync.RWMutex
	epSliceConfigs map[string]*epSliceConfig
}

func newEndpointSliceStore() *endpointSliceStore {
	return &endpointSliceStore{epSliceConfigs: make(map[string]*epSliceConfig)}
}

// epSliceConfig groups file-based config templates with the EndpointSlices that match them.
// Since file configs target services by namespace/name (not annotations), and a single service
// can have multiple EndpointSlices (N:1 relationship), we track all matching slices to generate
// one config per endpoint IP.
//
// The slices map uses UID as key for stable tracking across EndpointSlice updates, and
// shouldCollect indicates whether any slices are currently available for config generation.
type epSliceConfig struct {
	templates   []integration.Config
	slices      map[string]*discv1.EndpointSlice
	resolveMode endpointResolveMode
}

func newEpSliceConfig() *epSliceConfig {
	return &epSliceConfig{
		templates:   []integration.Config{},
		resolveMode: kubeEndpointResolveAuto, // default to auto mode
	}
}

func (s *epSliceConfig) shouldCollect() bool {
	return len(s.slices) > 0
}

// KubeEndpointSlicesFileConfigProvider generates endpoints checks from check configurations defined in files.
type KubeEndpointSlicesFileConfigProvider struct {
	sync.RWMutex
	upToDate bool
	store    *endpointSliceStore
}

// NewKubeEndpointSlicesFileConfigProvider returns a new KubeEndpointSlicesFileConfigProvider
func NewKubeEndpointSlicesFileConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, _ *telemetry.Store) (types.ConfigProvider, error) {
	templates, _, err := ReadConfigFiles(WithAdvancedADOnly)
	if err != nil {
		return nil, err
	}

	provider := &KubeEndpointSlicesFileConfigProvider{}
	provider.buildConfigStore(templates)
	if provider.store.isEmpty() {
		provider.setUpToDate(true)
		return provider, nil
	}

	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	epSliceInformer := ac.InformerFactory.Discovery().V1().EndpointSlices()
	if epSliceInformer == nil {
		return nil, errors.New("cannot get endpointslice informer")
	}

	if _, err := epSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    provider.addHandler,
		UpdateFunc: provider.updateHandler,
		DeleteFunc: provider.deleteHandler,
	}); err != nil {
		return nil, fmt.Errorf("cannot add event handler to endpointslice informer: %s", err)
	}

	return provider, nil
}

// Collect returns the check configurations defined in Yaml files.
// Only configs with advanced AD identifiers targeting kubernetes endpoints are handled by this collector.
func (p *KubeEndpointSlicesFileConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	p.setUpToDate(true)

	return p.store.generateConfigs(), nil
}

// IsUpToDate returns whether the config provider needs to be polled.
func (p *KubeEndpointSlicesFileConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	p.RLock()
	defer p.RUnlock()

	return p.upToDate, nil
}

// String returns a string representation of the KubeEndpointSlicesFileConfigProvider.
func (p *KubeEndpointSlicesFileConfigProvider) String() string {
	return names.KubeEndpointSlicesFile
}

// GetConfigErrors is not implemented for the KubeEndpointSlicesFileConfigProvider.
func (p *KubeEndpointSlicesFileConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return make(map[string]types.ErrorMsgSet)
}

func (p *KubeEndpointSlicesFileConfigProvider) setUpToDate(v bool) {
	p.Lock()
	defer p.Unlock()

	p.upToDate = v
}

func (p *KubeEndpointSlicesFileConfigProvider) addHandler(obj interface{}) {
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an EndpointSlice type, got: %T", obj)
		return
	}

	shouldUpdate := p.store.insertSlice(slice)
	if shouldUpdate {
		p.setUpToDate(false)
	}
}

func (p *KubeEndpointSlicesFileConfigProvider) updateHandler(old, new interface{}) {
	newSlice, ok := new.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an EndpointSlice type, got: %T", new)
		return
	}

	if !p.store.shouldHandle(newSlice) {
		return
	}

	oldSlice, ok := old.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an EndpointSlice type, got: %T", old)
		return
	}

	if !p.store.shouldHandle(oldSlice) {
		return
	}

	if !equality.Semantic.DeepEqual(newSlice.Endpoints, oldSlice.Endpoints) {
		p.deleteHandler(oldSlice)
		shouldUpdate := p.store.insertSlice(newSlice)
		if shouldUpdate {
			p.setUpToDate(false)
		}
	}
}

func (p *KubeEndpointSlicesFileConfigProvider) deleteHandler(obj interface{}) {
	slice, ok := obj.(*discv1.EndpointSlice)
	if !ok {
		log.Errorf("Expected an EndpointSlice type, got: %T", obj)
		return
	}

	p.store.deleteSlice(slice)
}

// buildConfigStore initializes the config templates store.
func (p *KubeEndpointSlicesFileConfigProvider) buildConfigStore(templates []integration.Config) {
	p.store = newEndpointSliceStore()
	for _, tpl := range templates {
		for _, advancedAD := range tpl.AdvancedADIdentifiers {
			if advancedAD.KubeEndpoints.IsEmpty() {
				continue
			}

			resolveMode := endpointResolveMode(advancedAD.KubeEndpoints.Resolve)
			if resolveMode == "" {
				resolveMode = kubeEndpointResolveAuto // default to auto mode
			}

			p.store.insertTemplate(epSliceID(advancedAD.KubeEndpoints.Namespace, advancedAD.KubeEndpoints.Name), tpl, resolveMode)
		}

		// Configuration defined using only CEL selectors
		if len(tpl.AdvancedADIdentifiers) == 0 && len(tpl.CELSelector.KubeEndpoints) > 0 {
			// Create matching program from CEL rules
			matchingProg, celADID, compileErr, recError := integration.CreateMatchingProgram(tpl.CELSelector)
			if celADID != adtypes.CelEndpointIdentifier {
				log.Errorf("CEL selector for template %s is not targeting endpoints", tpl.Name)
				continue
			}
			if compileErr != nil {
				log.Errorf("Failed to compile CEL selector for template %s: %v", tpl.Name, compileErr)
				continue
			}
			if recError != nil {
				log.Errorf("Failed to check rule recommendations for CEL selector for template %s: %v", tpl.Name, recError)
				continue
			}
			tpl.SetMatchingProgram(matchingProg)
			p.store.insertTemplate(celEndpointSliceID, tpl, kubeEndpointResolveAuto)
		}
	}
}

// matchesAnyCELTemplate checks if an endpointslice matches any CEL template.
func (s *endpointSliceStore) matchesAnyCELTemplate(slice *discv1.EndpointSlice) bool {
	celEpSliceConfig, celFound := s.epSliceConfigs[celEndpointSliceID]
	if !celFound || len(celEpSliceConfig.templates) == 0 {
		return false
	}

	serviceName := slice.Labels[kubernetesServiceNameLabelProvider]
	if serviceName == "" {
		return false
	}

	filterableEp := workloadfilter.CreateKubeEndpoint(serviceName, slice.Namespace, slice.GetAnnotations())
	for _, tpl := range celEpSliceConfig.templates {
		if tpl.IsMatched(filterableEp) {
			return true
		}
	}
	return false
}

// shouldHandle returns whether an endpointslice object should be tracked.
func (s *endpointSliceStore) shouldHandle(slice *discv1.EndpointSlice) bool {
	s.RLock()
	defer s.RUnlock()

	serviceName := slice.Labels[kubernetesServiceNameLabelProvider]
	if serviceName == "" {
		return false
	}

	// Check for AdvancedADIdentifer OR CEL Selector based match
	_, found := s.epSliceConfigs[epSliceID(slice.Namespace, serviceName)]
	return found || s.matchesAnyCELTemplate(slice)
}

// insertTemplate caches config templates with a specific resolve mode.
func (s *endpointSliceStore) insertTemplate(id string, tpl integration.Config, resolveMode endpointResolveMode) {
	s.Lock()
	defer s.Unlock()

	_, found := s.epSliceConfigs[id]
	if !found {
		s.epSliceConfigs[id] = newEpSliceConfig()
	}

	s.epSliceConfigs[id].templates = append(s.epSliceConfigs[id].templates, tpl)
	s.epSliceConfigs[id].resolveMode = resolveMode
}

// insertSlice caches the provided endpointslice object if it matches one of the tracked configs
// and prepares the config to be collected in the next Collect call.
// Returns false if the endpointslice object is irrelevant and discarded.
func (s *endpointSliceStore) insertSlice(slice *discv1.EndpointSlice) bool {
	s.Lock()
	defer s.Unlock()

	serviceName := slice.Labels[kubernetesServiceNameLabelProvider]
	if serviceName == "" {
		return false
	}

	shouldUpdate := false

	// Configuration defined using Advanced AD identifiers (exact namespace/name match)
	epSliceConfig, found := s.epSliceConfigs[epSliceID(slice.Namespace, serviceName)]
	if found {
		if epSliceConfig.slices == nil {
			epSliceConfig.slices = make(map[string]*discv1.EndpointSlice)
		}
		epSliceConfig.slices[string(slice.UID)] = slice
		shouldUpdate = true
	}

	// EndpointSlice matches any CEL template (CEL Selector based match)
	if s.matchesAnyCELTemplate(slice) {
		celEpSliceConfig := s.epSliceConfigs[celEndpointSliceID]
		if celEpSliceConfig.slices == nil {
			celEpSliceConfig.slices = make(map[string]*discv1.EndpointSlice)
		}
		celEpSliceConfig.slices[string(slice.UID)] = slice
		shouldUpdate = true
	}

	return shouldUpdate
}

// deleteSlice handles endpointslice objects deletion.
func (s *endpointSliceStore) deleteSlice(slice *discv1.EndpointSlice) {
	s.Lock()
	defer s.Unlock()

	celEpSliceConfig, celFound := s.epSliceConfigs[celEndpointSliceID]
	if celFound {
		delete(celEpSliceConfig.slices, string(slice.UID))
	}

	serviceName := slice.Labels[kubernetesServiceNameLabelProvider]
	if serviceName == "" {
		return
	}

	epSliceConfig, found := s.epSliceConfigs[epSliceID(slice.Namespace, serviceName)]
	if found {
		delete(epSliceConfig.slices, string(slice.UID))
	}
}

func (s *endpointSliceStore) isEmpty() bool {
	s.RLock()
	defer s.RUnlock()

	return len(s.epSliceConfigs) == 0
}

// generateConfigs transforms the cached config templates into collectable integration.Config objects
func (s *endpointSliceStore) generateConfigs() []integration.Config {
	s.Lock()
	defer s.Unlock()

	configs := []integration.Config{}
	for _, epSliceConfig := range s.epSliceConfigs {
		if epSliceConfig.shouldCollect() {
			for _, tpl := range epSliceConfig.templates {
				for _, slice := range epSliceConfig.slices {
					configs = append(configs, endpointSliceChecksFromTemplate(tpl, slice, epSliceConfig.resolveMode)...)
				}
			}
		}
	}
	return configs
}

// endpointSliceChecksFromTemplate resolves an integration.Config template based on the provided EndpointSlice object.
func endpointSliceChecksFromTemplate(tpl integration.Config, slice *discv1.EndpointSlice, resolveMode endpointResolveMode) []integration.Config {
	configs := []integration.Config{}
	if slice == nil {
		return configs
	}

	serviceName := slice.Labels[kubernetesServiceNameLabelProvider]
	if serviceName == "" {
		return configs
	}

	// Check resolve mode to know how we should process this endpoint
	resolveFunc := getEndpointResolveFuncForSlice(resolveMode, slice.Namespace, serviceName)

	for _, endpoint := range slice.Endpoints {
		for _, ip := range endpoint.Addresses {
			entity := apiserver.EntityForEndpoints(slice.Namespace, serviceName, ip)
			config := &integration.Config{
				ServiceID:               entity,
				Name:                    tpl.Name,
				Instances:               tpl.Instances,
				InitConfig:              tpl.InitConfig,
				MetricConfig:            tpl.MetricConfig,
				LogsConfig:              tpl.LogsConfig,
				CELSelector:             tpl.CELSelector,
				ADIdentifiers:           []string{entity},
				AdvancedADIdentifiers:   nil,
				ClusterCheck:            true,
				Provider:                names.KubeEndpointSlicesFile,
				Source:                  tpl.Source,
				IgnoreAutodiscoveryTags: tpl.IgnoreAutodiscoveryTags,
				CheckTagCardinality:     tpl.CheckTagCardinality,
			}

			if resolveFunc != nil {
				resolveFunc(config, endpoint)
			}
			configs = append(configs, *config)
		}
	}

	return configs
}

func epSliceID(ns, name string) string {
	return ns + "/" + name
}
