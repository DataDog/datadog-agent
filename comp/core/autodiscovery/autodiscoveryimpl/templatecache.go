// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// templateCache is a data structure to store configuration templates
type templateCache struct {
	adIDToDigests    map[string][]string           // map an AD identifier to all the configs that have it
	digestToADId     map[string][]string           // map a config digest to the list of AD identifiers it has
	digestToTemplate map[string]integration.Config // map a digest to the corresponding config object
	m                sync.RWMutex
}

// newTemplateCache creates a new cache
func newTemplateCache() *templateCache {
	return &templateCache{
		adIDToDigests:    map[string][]string{},
		digestToADId:     map[string][]string{},
		digestToTemplate: map[string]integration.Config{},
	}
}

// Set stores or updates a template in the cache
func (cache *templateCache) set(tpl integration.Config) error {
	// return an error if configuration has no AD identifiers
	if len(tpl.ADIdentifiers) == 0 {
		return fmt.Errorf("template has no AD identifiers, unable to store it in the cache")
	}

	cache.m.Lock()
	defer cache.m.Unlock()

	// compute the template digest once
	d := tpl.Digest()

	// do nothing if the template is already in cache
	if _, found := cache.digestToADId[d]; found {
		return nil
	}

	// store the template
	cache.digestToTemplate[d] = tpl
	cache.digestToADId[d] = tpl.ADIdentifiers
	for _, id := range tpl.ADIdentifiers {
		cache.adIDToDigests[id] = append(cache.adIDToDigests[id], d)
	}

	return nil
}

// Get retrieves a template from the cache
func (cache *templateCache) get(adID string) ([]integration.Config, error) {
	cache.m.RLock()
	defer cache.m.RUnlock()

	// do we know the identifier?
	if digests, found := cache.adIDToDigests[adID]; found {
		templates := []integration.Config{}
		for _, digest := range digests {
			templates = append(templates, cache.digestToTemplate[digest])
		}
		return templates, nil
	}

	return nil, fmt.Errorf("AD id %s not found in cache", adID)
}

// getUnresolvedTemplates returns all templates in the cache, in their unresolved
// state.
func (cache *templateCache) getUnresolvedTemplates() map[string][]integration.Config {
	tpls := make(map[string][]integration.Config)
	for d, config := range cache.digestToTemplate {
		ids := strings.Join(cache.digestToADId[d][:], ",")
		tpls[ids] = append(tpls[ids], config)
	}
	return tpls
}

// del removes a template from the cache
func (cache *templateCache) del(tpl integration.Config) error {
	// compute the digest once
	d := tpl.Digest()
	cache.m.Lock()
	defer cache.m.Unlock()

	// returns an error in case the template isn't there
	if _, found := cache.digestToADId[d]; !found {
		return fmt.Errorf("template not found in cache")
	}

	// remove the template
	delete(cache.digestToADId, d)
	delete(cache.digestToTemplate, d)

	// iterate through the AD identifiers for this config
	for _, id := range tpl.ADIdentifiers {
		digests := cache.adIDToDigests[id]
		// remove the template from id2templates
		if len(digests) == 1 && digests[0] == d {
			delete(cache.adIDToDigests, id)
		} else {
			for i, digest := range digests {
				if digest == d {
					cache.adIDToDigests[id] = append(digests[:i], digests[i+1:]...)
					break
				}
			}
		}
	}

	return nil
}
