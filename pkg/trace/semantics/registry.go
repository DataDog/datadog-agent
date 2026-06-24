// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	_ "embed" //nolint:revive
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
)

//go:embed mappings.json
var mappingsJSON []byte

type registryData struct {
	Version  string                    `json:"version"`
	Concepts map[string]ConceptMapping `json:"concepts"`
}

// EmbeddedRegistry loads semantic mappings from embedded JSON.
type EmbeddedRegistry struct {
	version  string
	mappings map[Concept][]TagInfo
}

var globalRegistry atomic.Pointer[Registry]

func init() {
	r := mustLoadRegistry()
	globalRegistry.Store(&r)
}

func mustLoadRegistry() Registry {
	r, err := NewEmbeddedRegistry()
	if err != nil {
		panic(fmt.Sprintf("failed to load semantic registry: %v", err))
	}
	return r
}

// DefaultRegistry returns the live semantic registry.
func DefaultRegistry() Registry {
	return *globalRegistry.Load()
}

// UpdateRegistry atomically replaces the live registry.
// Callers are responsible for refreshing any derived state (e.g. concentrator peer tag keys) after the swap.
// Called by RemoteConfigHandler only after successful validation.
func UpdateRegistry(r Registry) {
	globalRegistry.Store(&r)
}

// NewRegistryFromJSON constructs a Registry from raw JSON without affecting the live registry.
// Returns an error if the JSON is malformed or contains no concepts.
func NewRegistryFromJSON(data []byte) (Registry, error) {
	r := &EmbeddedRegistry{}
	if err := r.loadFromJSON(data); err != nil {
		return nil, err
	}
	if len(r.mappings) == 0 {
		return nil, errors.New("registry JSON contains no concepts")
	}
	return r, nil
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
	r.mappings = make(map[Concept][]TagInfo, len(rd.Concepts))
	for conceptName, mapping := range rd.Concepts {
		r.mappings[Concept(conceptName)] = mapping.Fallbacks
	}
	return nil
}

// GetAttributePrecedence returns the ordered attribute keys for a concept.
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

// RegistryEqual reports whether two registries are equal by comparing their
// Version() strings. Callers use this to decide whether to skip an
// UpdateRegistry call (and any downstream cache invalidation) when the
// publisher has pushed a payload that matches what is already live.
//
// TODO: this relies on the semantic-core publisher stamping a content-bound
// version (or a content hash) so that two registries with the same Version()
// truly have the same concept maps. Today the publisher uses a CI artifact
// version that is bumped on every build regardless of content changes, which
// makes this check pessimistic (we may swap even when concepts are unchanged).
// Coordinate with the semantic-core team to either make `version` content-
// bound or add a separate `content_hash` field to the payload; then switch
// this comparison to that field.
func RegistryEqual(a, b Registry) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Version() == b.Version()
}
