// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import "fmt"

// StaticCatalog provides artifact lookups from a fixed set of entries.
//
// TODO: Remove this implementation when PAR receives artifact catalogs through Remote Config.
type StaticCatalog struct {
	entries map[string]Descriptor
}

// staticCatalogEntries contains the temporary built-in authored-script catalog.
// Only descriptors with verified artifact coordinates should be added here.
var staticCatalogEntries = map[string]Descriptor{
	"com.datadoghq.authoredscripts.helm.addRepo": {
		Package: "com.datadoghq.authoredscripts.helm.addRepo",
		Version: "0.0.1",
		URL:     "oci://registry.ddbuild.io/dd-authored-scripts/dd-par-scripts-helm-add-repo@sha256:ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c",
		SHA256:  "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c",
	},
}

func NewStaticCatalog() *StaticCatalog {
	return &StaticCatalog{entries: staticCatalogEntries}
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
