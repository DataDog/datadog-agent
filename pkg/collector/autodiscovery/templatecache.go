package autodiscovery

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// TemplateCache is a data structure to store configuration templates
type TemplateCache struct {
	id2templates map[string][]check.Config
	template2ids map[string][]string
	m            sync.RWMutex
}

// NewTemplateCache creates a new cache
func NewTemplateCache() *TemplateCache {
	return &TemplateCache{
		id2templates: make(map[string][]check.Config, 0),
		template2ids: make(map[string][]string, 0),
	}
}

// Set stores or updates a template in the cache
func (cache *TemplateCache) Set(tpl check.Config) error {
	// return an error if configuration has no AD identifiers
	if len(tpl.ADIdentifiers) == 0 {
		return fmt.Errorf("template has no AD identifiers, unable to store it in the cache")
	}

	cache.m.Lock()
	defer cache.m.Unlock()

	// do nothing if the template is already in cache
	if _, found := cache.template2ids[tpl.Digest()]; found {
		return nil
	}

	cache.template2ids[tpl.Digest()] = tpl.ADIdentifiers
	for _, id := range tpl.ADIdentifiers {
		cache.id2templates[id] = append(cache.id2templates[id], tpl)
	}

	return nil
}

// Get retrieves a template from the cache
func (cache *TemplateCache) Get(adID string) ([]check.Config, error) {
	cache.m.RLock()
	defer cache.m.RUnlock()

	if templates, found := cache.id2templates[adID]; found {
		return templates, nil
	}

	return nil, fmt.Errorf("Autodiscovery id not found in cache")
}

// Del removes a template from the cache
func (cache *TemplateCache) Del(tpl check.Config) error {
	// compute the digest once
	tplDigest := tpl.Digest()

	cache.m.Lock()
	defer cache.m.Unlock()

	// returns an error in case the template isn't there
	if _, found := cache.template2ids[tplDigest]; !found {
		return fmt.Errorf("template not found in cache")
	}

	// remove the template from template2ids
	delete(cache.template2ids, tplDigest)

	// iterate through the AD identifiers for this config
	for _, id := range tpl.ADIdentifiers {
		tpls := cache.id2templates[id]
		// remove the template from id2templates
		for i, template := range tpls {
			if template.Equal(&tpl) {
				cache.id2templates[id] = append(tpls[:i], tpls[i+1:]...)
				break
			}
		}
	}

	return nil
}
