// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	_ "embed" //nolint:revive
	"encoding/json"
	"fmt"
)

//go:embed mappings.json
var mappingsJSON []byte

// registryData represents the JSON structure of the mappings file.
type registryData struct {
	Version  string                    `json:"version"`
	Concepts map[string]ConceptMapping `json:"concepts"`
}

// EmbeddedRegistry is a Registry implementation that loads semantic mappings
// from the embedded JSON configuration file. The registry is loaded at
// construction time and is safe for concurrent use.
type EmbeddedRegistry struct {
	version  string
	mappings map[Concept][]TagInfo
}

// globalRegistry is the default registry instance using embedded mappings.
// Initialized at package load time; panics if loading fails.
var globalRegistry = mustLoadRegistry()

func mustLoadRegistry() *EmbeddedRegistry {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		panic(fmt.Sprintf("failed to load semantic registry: %v", err))
	}
	return r
}

// DefaultRegistry returns the default semantic registry with embedded mappings.
func DefaultRegistry() Registry {
	return globalRegistry
}

// NewEmbeddedRegistry creates a new EmbeddedRegistry from the embedded JSON mappings.
// Returns an error if the embedded mappings fail to load.
func NewEmbeddedRegistry() (*EmbeddedRegistry, error) {
	r := &EmbeddedRegistry{}
	if err := r.loadFromJSON(mappingsJSON); err != nil {
		return nil, fmt.Errorf("failed to load embedded mappings: %w", err)
	}
	return r, nil
}

// NewRegistryFromJSON creates a new EmbeddedRegistry from custom JSON data.
// Useful for testing or when loading mappings from an external source.
func NewRegistryFromJSON(data []byte) (*EmbeddedRegistry, error) {
	r := &EmbeddedRegistry{}
	if err := r.loadFromJSON(data); err != nil {
		return nil, err
	}
	return r, nil
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
		r.mappings[concept] = mapping.Fallbacks
	}

	return nil
}

// GetAttributePrecedence returns the ordered list of attribute keys to check for a given semantic concept.
// First key in the list has highest precedence.
// Returns nil if the concept is not found.
func (r *EmbeddedRegistry) GetAttributePrecedence(concept Concept) []TagInfo {
	return r.mappings[concept]
}

// GetAllEquivalences returns all semantic equivalences as a map from concept to the ordered list of equivalent attribute keys.
func (r *EmbeddedRegistry) GetAllEquivalences() map[Concept][]TagInfo {
	// Return a copy to prevent external modification
	result := make(map[Concept][]TagInfo, len(r.mappings))
	for k, v := range r.mappings {
		result[k] = v
	}
	return result
}

// Version returns the semantic registry version string.
func (r *EmbeddedRegistry) Version() string {
	return r.version
}
