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

type registryData struct {
	Version  string                    `json:"version"`
	Concepts map[string]ConceptMapping `json:"concepts"`
}

// EmbeddedRegistry loads semantic mappings from embedded JSON.
// mappings is indexed by Concept (int), so GetAttributePrecedence is a single
// array index with no hashing or key comparison.
type EmbeddedRegistry struct {
	version  string
	mappings [conceptCount][]TagInfo
}

var globalRegistry = mustLoadRegistry()

func mustLoadRegistry() *EmbeddedRegistry {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		panic(fmt.Sprintf("failed to load semantic registry: %v", err))
	}
	return r
}

// DefaultRegistry returns the default semantic registry.
func DefaultRegistry() Registry {
	return globalRegistry
}

// NewEmbeddedRegistry creates a registry from embedded JSON mappings.
func NewEmbeddedRegistry() (*EmbeddedRegistry, error) {
	r := &EmbeddedRegistry{}
	if err := r.loadFromJSON(mappingsJSON); err != nil {
		return nil, fmt.Errorf("failed to load embedded mappings: %w", err)
	}
	return r, nil
}

func (r *EmbeddedRegistry) loadFromJSON(data []byte) error {
	var rd registryData
	if err := json.Unmarshal(data, &rd); err != nil {
		return err
	}
	r.version = rd.Version

	// Build a reverse lookup from canonical JSON name → Concept ID.
	nameToID := make(map[string]Concept, conceptCount)
	for id := Concept(0); id < conceptCount; id++ {
		nameToID[conceptNames[id]] = id
	}

	for conceptName, mapping := range rd.Concepts {
		if id, ok := nameToID[conceptName]; ok {
			r.mappings[id] = mapping.Fallbacks
		}
	}
	return nil
}

// GetAttributePrecedence returns the ordered attribute keys for a concept.
// Out-of-range concepts return nil.
func (r *EmbeddedRegistry) GetAttributePrecedence(concept Concept) []TagInfo {
	if concept < 0 || concept >= conceptCount {
		return nil
	}
	return r.mappings[concept]
}

// GetAllEquivalences returns all semantic equivalences as a map from concept to the ordered list of equivalent attribute keys.
func (r *EmbeddedRegistry) GetAllEquivalences() map[Concept][]TagInfo {
	result := make(map[Concept][]TagInfo, conceptCount)
	for id := Concept(0); id < conceptCount; id++ {
		if tags := r.mappings[id]; tags != nil {
			result[id] = tags
		}
	}
	return result
}

// Version returns the semantic registry version string.
func (r *EmbeddedRegistry) Version() string {
	return r.version
}
