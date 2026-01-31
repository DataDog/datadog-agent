// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	_ "embed" //nolint:revive
	"encoding/json"
	"sync"
)

//go:embed mappings.json
var mappingsJSON []byte

// registryData represents the JSON structure of the mappings file.
type registryData struct {
	Version  string                    `json:"version"`
	Concepts map[string]ConceptMapping `json:"concepts"`
}

// EmbeddedRegistry is a Registry implementation that loads semantic mappings
// from the embedded JSON configuration file. This registry is initialized
// lazily on first access and is safe for concurrent use.
type EmbeddedRegistry struct {
	once     sync.Once
	version  string
	mappings map[Concept][]TagInfo
	loadErr  error
}

// globalRegistry is the default registry instance using embedded mappings.
var globalRegistry = &EmbeddedRegistry{}

// DefaultRegistry returns the default semantic registry with embedded mappings.
// The registry is initialized lazily on first access.
func DefaultRegistry() Registry {
	return globalRegistry
}

// NewEmbeddedRegistry creates a new EmbeddedRegistry from the embedded JSON mappings.
func NewEmbeddedRegistry() *EmbeddedRegistry {
	return &EmbeddedRegistry{}
}

// NewRegistryFromJSON creates a new EmbeddedRegistry from custom JSON data -- useful for testing or when loading mappings from an external source.
func NewRegistryFromJSON(data []byte) (*EmbeddedRegistry, error) {
	r := &EmbeddedRegistry{}
	if err := r.loadFromJSON(data); err != nil {
		return nil, err
	}
	// Mark as already loaded to prevent the sync.Once from loading embedded data
	r.once.Do(func() {})
	return r, nil
}

// load initializes the registry from the embedded JSON mappings.
func (r *EmbeddedRegistry) load() {
	r.once.Do(func() {
		r.loadErr = r.loadFromJSON(mappingsJSON)
	})
}

// loadFromJSON parses JSON data and populates the registry.
func (r *EmbeddedRegistry) loadFromJSON(data []byte) error {
	var rd registryData
	if err := json.Unmarshal(data, &rd); err != nil {
		return err
	}

	r.version = rd.Version
	r.mappings = make(map[Concept][]TagInfo, len(rd.Concepts))

	for conceptName, mapping := range rd.Concepts {
		concept := Concept(conceptName)
		// If there are no fallbacks, use the canonical name as the only fallback
		if len(mapping.Fallbacks) == 0 {
			r.mappings[concept] = []TagInfo{
				{Name: mapping.Canonical, Provider: ProviderDatadog},
			}
		} else {
			r.mappings[concept] = mapping.Fallbacks
		}
	}

	return nil
}

// GetAttributePrecedence returns the ordered list of attribute keys to check for a given semantic concept.
// First key in the list has highest precedence.
// Returns nil if the concept is not found or if loading failed.
func (r *EmbeddedRegistry) GetAttributePrecedence(concept Concept) []TagInfo {
	r.load()
	if r.loadErr != nil {
		return nil
	}
	return r.mappings[concept]
}

// GetAllEquivalences returns all semantic equivalences as a map from concept to the ordered list of equivalent attribute keys.
// Returns nil if loading failed.
func (r *EmbeddedRegistry) GetAllEquivalences() map[Concept][]TagInfo {
	r.load()
	if r.loadErr != nil {
		return nil
	}
	// Return a copy to prevent external modification
	result := make(map[Concept][]TagInfo, len(r.mappings))
	for k, v := range r.mappings {
		result[k] = v
	}
	return result
}

// Version returns the semantic registry version string.
func (r *EmbeddedRegistry) Version() string {
	r.load()
	if r.loadErr != nil {
		return ""
	}
	return r.version
}

// LoadError returns any error that occurred during loading.
// This is useful for debugging and logging.
func (r *EmbeddedRegistry) LoadError() error {
	r.load()
	return r.loadErr
}

// GetTagNames returns just the attribute names (without metadata) for a concept.
// This is a convenience method for simple lookups.
func (r *EmbeddedRegistry) GetTagNames(concept Concept) []string {
	tags := r.GetAttributePrecedence(concept)
	if tags == nil {
		return nil
	}
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Name
	}
	return names
}
