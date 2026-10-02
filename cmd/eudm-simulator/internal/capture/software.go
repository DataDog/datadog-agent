// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/inventory/software"
)

// Software owns the complete snapshot without changing names, versions,
// publishers, paths, deployment states, or historical installation dates.
func (*Normalizer) Software(entries []software.Entry) []software.Entry {
	owned := slices.Clone(entries)
	for i := range owned {
		owned[i].InstallPaths = slices.Clone(entries[i].InstallPaths)
	}
	return owned
}
