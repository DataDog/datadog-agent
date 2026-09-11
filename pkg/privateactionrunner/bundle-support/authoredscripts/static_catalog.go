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
		URL:     "oci://install.datad0g.com/com.datadoghq.authoredscripts.helm.addrepo-package:0.0.1-1",
		SHA256:  "cd4ef4c7ef5d394a407d05a1d60f6efac64aa3d507124f55f4385e1b5c6e5bfe",
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
