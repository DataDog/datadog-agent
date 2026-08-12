// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package artifacts

import (
	"errors"
	"fmt"
)

// StaticCatalog provides artifact lookups from a fixed set of entries.
//
// TODO: Remove this implementation when PAR receives artifact catalogs through Remote Config.
type StaticCatalog struct {
	entries map[string]map[Platform]Descriptor
}

func NewStaticCatalog(entries map[string]map[Platform]Descriptor) *StaticCatalog {
	catalogEntries := make(map[string]map[Platform]Descriptor, len(entries))
	for key, platformEntries := range entries {
		catalogEntries[key] = make(map[Platform]Descriptor, len(platformEntries))
		for platform, descriptor := range platformEntries {
			descriptor.Platform = platform
			catalogEntries[key][platform] = descriptor
		}
	}
	return &StaticCatalog{entries: catalogEntries}
}

func (c *StaticCatalog) Lookup(key string, platform Platform) (Descriptor, error) {
	if c == nil {
		return Descriptor{}, errors.New("static artifact catalog is not configured")
	}
	platformEntries, found := c.entries[key]
	if !found {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrNotFound, key)
	}
	descriptor, found := platformEntries[platform]
	if !found {
		return Descriptor{}, fmt.Errorf("%w: %q for %s/%s", ErrNotFound, key, platform.OS, platform.Arch)
	}
	return descriptor, nil
}
