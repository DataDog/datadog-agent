// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"crypto/sha256"
	_ "embed" //nolint:revive
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
)

//go:embed mappings.json
var mappingsJSON []byte

const (
	// SourceEmbedded marks a registry loaded from the embedded mappings.json.
	SourceEmbedded = "embedded"
	// SourceRemoteConfig marks a registry delivered via Remote Configuration.
	SourceRemoteConfig = "remote-config"
)

type registryMetadata struct {
	ContentHash string `json:"content_hash"`
}

type registryData struct {
	Version  string                    `json:"version"`
	Metadata registryMetadata          `json:"metadata"`
	Concepts map[string]ConceptMapping `json:"concepts"`
}

// EmbeddedRegistry loads semantic mappings from embedded JSON.
type EmbeddedRegistry struct {
	version     string
	hash        string
	fingerprint string
	source      string
	mappings    map[Concept][]TagInfo
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
// Returns an error if the JSON is malformed, contains no concepts, or is missing metadata.content_hash.
func NewRegistryFromJSON(data []byte) (Registry, error) {
	r := &EmbeddedRegistry{source: SourceRemoteConfig}
	if err := r.loadFromJSON(data); err != nil {
		return nil, err
	}
	return r, nil
}

// NewEmbeddedRegistry creates a registry from embedded JSON mappings.
func NewEmbeddedRegistry() (*EmbeddedRegistry, error) {
	r := &EmbeddedRegistry{source: SourceEmbedded}
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
	if len(rd.Concepts) == 0 {
		return errors.New("registry JSON contains no concepts")
	}
	if rd.Metadata.ContentHash == "" {
		return errors.New("registry JSON missing metadata.content_hash")
	}
	r.version = rd.Version
	r.hash = rd.Metadata.ContentHash
	r.mappings = make(map[Concept][]TagInfo, len(rd.Concepts))
	for conceptName, mapping := range rd.Concepts {
		// Canonical is deliberately dropped because it does not currently affect
		// registry lookups. It remains covered by the raw-payload fingerprint.
		r.mappings[Concept(conceptName)] = mapping.Fallbacks
	}
	sum := sha256.Sum256(data)
	r.fingerprint = hex.EncodeToString(sum[:])
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

// ContentHash returns the producer-declared registry version label verbatim.
// It is not an integrity check or a key for invalidating derived state.
func (r *EmbeddedRegistry) ContentHash() string {
	return r.hash
}

// Fingerprint returns the locally computed identity of the payload this registry
// was built from. It is the correct key for deciding registry publication and
// invalidating registry-derived state. It is never published, persisted, or
// compared against a producer-supplied hash, and carries no cross-process or
// cross-version meaning.
func (r *EmbeddedRegistry) Fingerprint() string {
	return r.fingerprint
}

// Source reports where the registry came from (SourceEmbedded or SourceRemoteConfig).
func (r *EmbeddedRegistry) Source() string {
	return r.source
}

// RegistryEqual reports whether two registries were built from identical
// payload bytes. It is the key for both publishing a registry and invalidating
// registry-derived state. It is presentation-sensitive, so cosmetically
// reformatted payloads compare unequal; this is accepted because
// over-invalidating is cheap while under-invalidating is a bug.
func RegistryEqual(a, b Registry) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Fingerprint() == b.Fingerprint()
}
