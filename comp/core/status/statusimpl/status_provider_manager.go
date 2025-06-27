// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"hash/fnv"
	"sort"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

// statusProviderManager is a manager for status providers.
// It handles both static and dynamic providers, sorting them and caching the results.
// Change detection for dynamic providers are based on header indexes and provider names.
type statusProviderManager struct {
	log log.Component

	// Static providers
	commonHeaderProvider status.HeaderProvider
	headerProviders      []status.HeaderProvider
	statusProviders      []status.Provider

	// Dynamic providers
	headerProvidersGetters []func() []status.HeaderProvider
	providersGetters       []func() []status.Provider

	// internal cache
	_sortedSectionNames       []string
	_sortedHeaderProviders    []status.HeaderProvider
	_sortedProvidersBySection map[string][]status.Provider

	// last dynamic providers hash
	_lastDynamicProvidersHash uint64
}

func newProviderGetter(
	log log.Component,
	commonHeaderProvider status.HeaderProvider,
	headerProviders []status.HeaderProvider,
	statusProviders []status.Provider,
	headerProvidersGetters []func() []status.HeaderProvider,
	providersGetters []func() []status.Provider,
) statusProviderManager {
	providerGetter := statusProviderManager{
		log:                    log,
		commonHeaderProvider:   commonHeaderProvider,
		headerProviders:        headerProviders,
		statusProviders:        statusProviders,
		headerProvidersGetters: headerProvidersGetters,
		providersGetters:       providersGetters,
	}

	// Initialize the internal cache
	providerGetter.sortProviders(true)

	return providerGetter
}

// sortProviders sorts and caches the status and header providers managed by the statusProviderManager.
// It determines whether a resort is necessary based on the presence of dynamic providers or a change
// in the set of dynamic providers, detected via a hash of their names and sections. If no changes are
// detected and resorting is not forced, the cached sorted providers are reused. Otherwise, it performs
// the following steps:
//   - Computes a hash of the current dynamic providers to detect changes.
//   - Sorts section names alphabetically, ensuring the "collector" section appears first if present.
//   - Sorts providers within each section alphabetically by name.
//   - Sorts header providers by their index, ensuring the common header provider appears first.
//   - Updates the internal cache with the newly sorted providers and section names.
//
// The function is optimized to avoid unnecessary sorting and supports both static and dynamic providers.
func (p *statusProviderManager) sortProviders(forceResort bool) {
	// If we are not forcing a resort and there is no dynamic providers,
	// we can return the cached sorted providers
	if !forceResort && len(p.providersGetters) == 0 && len(p.headerProvidersGetters) == 0 {
		p.log.Debug("No dynamic providers found, returning cached sorted providers")
		return
	}

	// Checking if the dynamic providers have changed by computing a hash of the providers
	// by iteratevely computing fnv hash of the provider names and sections
	hasher := fnv.New64a()

	computedHeaderProviders := [][]status.HeaderProvider{}
	computedProviders := [][]status.Provider{}

	for _, getter := range p.headerProvidersGetters {
		currentHeaderProviders := []status.HeaderProvider{}
		for _, provider := range getter() {
			hasher.Write([]byte{byte(provider.Index())})
			hasher.Write([]byte(provider.Name()))
			currentHeaderProviders = append(currentHeaderProviders, provider)
		}
		computedHeaderProviders = append(computedHeaderProviders, currentHeaderProviders)
	}
	for _, getter := range p.providersGetters {
		currentProvider := []status.Provider{}
		for _, provider := range getter() {
			hasher.Write([]byte(provider.Name()))
			currentProvider = append(currentProvider, provider)
		}
		computedProviders = append(computedProviders, currentProvider)
	}

	// compare the hash of the current providers with the last one
	currentDynamicProvidersHash := hasher.Sum64()
	if p._lastDynamicProvidersHash == currentDynamicProvidersHash {
		// If the hash is the same, we can return the cached sorted providers
		p.log.Debugf("Dynamic providers have not changed (%v == %v), returning cached sorted providers", currentDynamicProvidersHash, p._lastDynamicProvidersHash)
		return
	}
	p.log.Debugf("Dynamic providers have changed (%v != %v), resorting providers", currentDynamicProvidersHash, p._lastDynamicProvidersHash)
	// If the hash is different, we need to re-sort the providers
	p._lastDynamicProvidersHash = currentDynamicProvidersHash

	// Sections are sorted by name
	// The exception is the collector section. We want that to be the first section to be displayed
	// We manually insert the collector section in the first place after sorting them alphabetically
	sortedSectionNames := []string{}
	collectorSectionPresent := false

	// Get all providers from the static and dynamic providers
	providers := p.statusProviders
	for _, provider := range computedProviders {
		providers = append(providers, provider...)
	}

	for _, provider := range providers {
		if provider.Section() == status.CollectorSection && !collectorSectionPresent {
			collectorSectionPresent = true
		}

		if !present(provider.Section(), sortedSectionNames) && provider.Section() != status.CollectorSection {
			sortedSectionNames = append(sortedSectionNames, strings.ToLower(provider.Section()))
		}
	}

	sort.Strings(sortedSectionNames)

	if collectorSectionPresent {
		sortedSectionNames = append([]string{status.CollectorSection}, sortedSectionNames...)
	}

	// Providers of each section are sort alphabetically by name
	// Section names are stored lower case
	sortedProvidersBySection := map[string][]status.Provider{}
	for _, provider := range providers {
		lowerSectionName := strings.ToLower(provider.Section())
		providers := sortedProvidersBySection[lowerSectionName]
		sortedProvidersBySection[lowerSectionName] = append(providers, provider)
	}
	for section, providers := range sortedProvidersBySection {
		sortedProvidersBySection[section] = sortByName(providers)
	}

	// Header providers are sorted by index
	// We manually insert the common header provider in the first place after sorting is done
	sortedHeaderProviders := p.headerProviders
	for _, headerProvider := range computedHeaderProviders {
		sortedHeaderProviders = append(sortedHeaderProviders, headerProvider...)
	}

	sort.SliceStable(sortedHeaderProviders, func(i, j int) bool {
		return sortedHeaderProviders[i].Index() < sortedHeaderProviders[j].Index()
	})

	sortedHeaderProviders = append([]status.HeaderProvider{p.commonHeaderProvider}, sortedHeaderProviders...)

	// Update the internal cache with the sorted providers
	p._sortedSectionNames = sortedSectionNames
	p._sortedProvidersBySection = sortedProvidersBySection
	p._sortedHeaderProviders = sortedHeaderProviders
}

func sortByName(providers []status.Provider) []status.Provider {
	sort.SliceStable(providers, func(i, j int) bool {
		return providers[i].Name() < providers[j].Name()
	})

	return providers
}

// SortedProviders returns the sorted providers by section.
func (p *statusProviderManager) SortedHeaderProviders() []status.HeaderProvider {
	p.sortProviders(false)
	return p._sortedHeaderProviders
}

// SortedSectionNames returns the sorted section names.
func (p *statusProviderManager) SortedSectionNames() []string {
	p.sortProviders(false)
	return p._sortedSectionNames
}

// SortedProvidersBySection returns the sorted providers by section.
func (p *statusProviderManager) SortedProvidersBySection() map[string][]status.Provider {
	p.sortProviders(false)
	return p._sortedProvidersBySection
}
