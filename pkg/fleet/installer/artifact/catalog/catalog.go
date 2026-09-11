// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package catalog parses and indexes immutable artifacts published through the
// updater catalog schema. It is independent of Remote Config delivery.
package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/opencontainers/go-digest"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact"
)

var (
	// ErrNotReady is returned before a LiveCatalog receives its first snapshot.
	ErrNotReady = errors.New("artifact catalog is not ready")
	// ErrNotFound is returned when no descriptor matches a selector.
	ErrNotFound = errors.New("artifact is not present in the catalog")
	// ErrAmbiguous is returned when more than one descriptor matches a selector.
	ErrAmbiguous = errors.New("artifact selector matches multiple catalog entries")
	// ErrNotAuthorized is returned when an expected descriptor is no longer in
	// the current catalog.
	ErrNotAuthorized = errors.New("artifact is not authorized by the current catalog")
)

type catalogDocument struct {
	Packages []catalogEntry `json:"packages"`
}

type catalogEntry struct {
	Package  string `json:"package"`
	Version  string `json:"version"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

// Document is one independently delivered catalog fragment. Name is used only
// for deterministic processing and useful errors.
type Document struct {
	Name     string
	Contents []byte
}

// Selector describes a catalog lookup. Empty Version, OS, or Architecture
// fields are wildcards. Package is always required.
type Selector struct {
	Package      string
	Version      string
	OS           string
	Architecture string
}

// Snapshot is an immutable, validated view of a set of catalog documents.
type Snapshot struct {
	entries   []artifact.Descriptor
	byPackage map[string][]artifact.Descriptor
}

// Parse validates documents as one complete catalog snapshot. Either every
// document is accepted or no Snapshot is returned.
func Parse(documents []Document) (*Snapshot, error) {
	documents = slices.Clone(documents)
	slices.SortFunc(documents, func(a, b Document) int { return strings.Compare(a.Name, b.Name) })

	entries := make([]artifact.Descriptor, 0)
	seen := make(map[string]string)
	for _, document := range documents {
		if document.Name == "" {
			return nil, errors.New("catalog document name is required")
		}
		var payload catalogDocument
		if err := json.Unmarshal(document.Contents, &payload); err != nil {
			return nil, fmt.Errorf("could not decode catalog document %q: %w", document.Name, err)
		}
		if payload.Packages == nil {
			return nil, fmt.Errorf("catalog document %q does not contain packages", document.Name)
		}
		for index, entry := range payload.Packages {
			descriptor, err := parseEntry(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid entry %d in catalog document %q: %w", index, document.Name, err)
			}
			key := descriptorKey(descriptor)
			if previousDocument, found := seen[key]; found {
				return nil, fmt.Errorf("duplicate catalog entry %s in documents %q and %q", key, previousDocument, document.Name)
			}
			seen[key] = document.Name
			entries = append(entries, descriptor)
		}
	}

	slices.SortFunc(entries, compareDescriptors)
	byPackage := make(map[string][]artifact.Descriptor)
	for _, descriptor := range entries {
		byPackage[descriptor.Package] = append(byPackage[descriptor.Package], descriptor)
	}
	return &Snapshot{entries: entries, byPackage: byPackage}, nil
}

// ParseMap is a convenience wrapper for callers, such as Remote Config
// adapters, that naturally receive documents keyed by configuration path.
func ParseMap(documents map[string][]byte) (*Snapshot, error) {
	list := make([]Document, 0, len(documents))
	for documentName, contents := range documents {
		list = append(list, Document{Name: documentName, Contents: slices.Clone(contents)})
	}
	return Parse(list)
}

// Entries returns a copy of all entries in deterministic order.
func (s *Snapshot) Entries() []artifact.Descriptor {
	if s == nil {
		return nil
	}
	return slices.Clone(s.entries)
}

// Select returns the only descriptor matching selector. When a platform is
// requested, an exact platform entry takes precedence over a generic entry.
func (s *Snapshot) Select(selector Selector) (artifact.Descriptor, error) {
	if s == nil {
		return artifact.Descriptor{}, ErrNotReady
	}
	if selector.Package == "" {
		return artifact.Descriptor{}, errors.New("artifact selector package is required")
	}
	candidates := s.byPackage[selector.Package]
	var matches []artifact.Descriptor
	bestPlatformSpecificity := -1
	for _, candidate := range candidates {
		if selector.Version != "" && candidate.Version != selector.Version {
			continue
		}
		if !matchesPlatform(candidate.Platform.OS, selector.OS) || !matchesPlatform(candidate.Platform.Architecture, selector.Architecture) {
			continue
		}
		specificity := platformSpecificity(candidate.Platform, selector)
		if specificity < bestPlatformSpecificity {
			continue
		}
		if specificity > bestPlatformSpecificity {
			matches = matches[:0]
			bestPlatformSpecificity = specificity
		}
		matches = append(matches, candidate)
	}
	switch len(matches) {
	case 0:
		return artifact.Descriptor{}, fmt.Errorf("%w: package %q", ErrNotFound, selector.Package)
	case 1:
		return matches[0], nil
	default:
		return artifact.Descriptor{}, fmt.Errorf("%w: package %q", ErrAmbiguous, selector.Package)
	}
}

// Contains reports whether the exact descriptor is part of the Snapshot.
func (s *Snapshot) Contains(expected artifact.Descriptor) bool {
	if s == nil {
		return false
	}
	for _, candidate := range s.byPackage[expected.Package] {
		if descriptorsEqual(candidate, expected) {
			return true
		}
	}
	return false
}

// LiveCatalog atomically publishes complete, validated snapshots. A failed
// parse must not be passed to Replace, which preserves last-known-good state.
type LiveCatalog struct {
	mu       sync.RWMutex
	snapshot *Snapshot
}

// Replace activates snapshot. Snapshot is immutable and may be shared safely.
func (c *LiveCatalog) Replace(snapshot *Snapshot) error {
	if c == nil {
		return errors.New("live artifact catalog is required")
	}
	if snapshot == nil {
		return errors.New("artifact catalog snapshot is required")
	}
	c.mu.Lock()
	c.snapshot = snapshot
	c.mu.Unlock()
	return nil
}

// Select looks up a descriptor in the current Snapshot.
func (c *LiveCatalog) Select(selector Selector) (artifact.Descriptor, error) {
	if c == nil {
		return artifact.Descriptor{}, ErrNotReady
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshot.Select(selector)
}

// WithAuthorized runs use while holding a consistent view of the current
// catalog, but only if it still contains expected. use should perform only the
// authorization-sensitive transition, such as starting a process; it must not
// perform downloads or other long-running work while the read lock is held.
func (c *LiveCatalog) WithAuthorized(expected artifact.Descriptor, use func() error) error {
	if c == nil {
		return ErrNotReady
	}
	if use == nil {
		return errors.New("authorized artifact operation is required")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.snapshot == nil {
		return ErrNotReady
	}
	if !c.snapshot.Contains(expected) {
		return fmt.Errorf("%w: package %q version %q digest %q", ErrNotAuthorized, expected.Package, expected.Version, expected.Digest)
	}
	return use()
}

func parseEntry(entry catalogEntry) (artifact.Descriptor, error) {
	if entry.SHA256 == "" {
		return artifact.Descriptor{}, errors.New("SHA-256 digest is required")
	}
	artifactDigest := digest.NewDigestFromEncoded(digest.SHA256, entry.SHA256)
	descriptor := artifact.Descriptor{
		Package:   entry.Package,
		Version:   entry.Version,
		Reference: entry.URL,
		Digest:    artifactDigest,
		Size:      entry.Size,
		Platform: artifact.Platform{
			OS:           entry.Platform,
			Architecture: entry.Arch,
		},
	}
	if err := descriptor.Validate(); err != nil {
		return artifact.Descriptor{}, err
	}
	parsedURL, err := url.Parse(descriptor.Reference)
	if err != nil {
		return artifact.Descriptor{}, fmt.Errorf("could not parse artifact URL: %w", err)
	}
	if parsedURL.Scheme == "oci" {
		if parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
			return artifact.Descriptor{}, errors.New("artifact OCI URL must not contain user information, a query, or a fragment")
		}
		reference, err := name.NewDigest(strings.TrimPrefix(descriptor.Reference, "oci://"), name.StrictValidation)
		if err != nil {
			return artifact.Descriptor{}, fmt.Errorf("artifact URL must contain a valid immutable OCI digest: %w", err)
		}
		if reference.DigestStr() != descriptor.Digest.String() {
			return artifact.Descriptor{}, fmt.Errorf("artifact URL digest %q does not match catalog digest %q", reference.DigestStr(), descriptor.Digest)
		}
	}
	return descriptor, nil
}

func matchesPlatform(entry, requested string) bool {
	return entry == "" || requested == "" || entry == requested
}

func platformSpecificity(platform artifact.Platform, selector Selector) int {
	if selector.OS != "" && platform.OS != "" {
		return 1
	}
	return 0
}

func descriptorKey(d artifact.Descriptor) string {
	return strings.Join([]string{d.Package, d.Version, d.Platform.OS, d.Platform.Architecture}, "/")
}

func compareDescriptors(a, b artifact.Descriptor) int {
	for _, comparison := range []int{
		strings.Compare(a.Package, b.Package),
		strings.Compare(a.Version, b.Version),
		strings.Compare(a.Platform.OS, b.Platform.OS),
		strings.Compare(a.Platform.Architecture, b.Platform.Architecture),
		strings.Compare(a.Digest.String(), b.Digest.String()),
	} {
		if comparison != 0 {
			return comparison
		}
	}
	return 0
}

func descriptorsEqual(a, b artifact.Descriptor) bool {
	return a.Package == b.Package &&
		a.Version == b.Version &&
		a.Reference == b.Reference &&
		a.Digest == b.Digest &&
		a.Size == b.Size &&
		a.Platform == b.Platform
}
