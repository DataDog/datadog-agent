// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"fmt"
	"maps"
)

// StaticCatalog provides artifact lookups from a fixed set of entries.
type StaticCatalog struct {
	entries map[string]Descriptor
}

// staticCatalogEntries contains the temporary built-in authored-script catalog.
// Only descriptors with verified artifact coordinates should be added here.
var staticCatalogEntries = map[string]Descriptor{
	"com.datadoghq.authoredscripts.helm.addRepo": {
		Package: "com.datadoghq.authoredscripts.helm.addrepo",
		Version: "0.0.1",
		URL:     "oci://install.datad0g.com/com.datadoghq.authoredscripts.helm.addrepo-package@sha256:cd4ef4c7ef5d394a407d05a1d60f6efac64aa3d507124f55f4385e1b5c6e5bfe",
		SHA256:  "cd4ef4c7ef5d394a407d05a1d60f6efac64aa3d507124f55f4385e1b5c6e5bfe",
	},
}

// NewStaticCatalog creates a catalog containing the built-in fixed entries.
func NewStaticCatalog() *StaticCatalog {
	return &StaticCatalog{entries: maps.Clone(staticCatalogEntries)}
}

// Lookup returns the descriptor configured for key.
func (c *StaticCatalog) Lookup(key string) (Descriptor, error) {
	if c != nil {
		if descriptor, ok := c.entries[key]; ok {
			return descriptor, nil
		}
	}
	return Descriptor{}, fmt.Errorf("%w: %q", ErrPackageNotConfigured, key)
}

// WithAuthorized runs use when the fixed catalog still contains descriptor.
func (c *StaticCatalog) WithAuthorized(descriptor Descriptor, use func() error) error {
	if use == nil {
		return fmt.Errorf("authorized authored-script operation is required")
	}
	if c != nil {
		for _, current := range c.entries {
			if current == descriptor {
				return use()
			}
		}
	}
	return fmt.Errorf("%w: package %q version %q", ErrPackageNotConfigured, descriptor.Package, descriptor.Version)
}
