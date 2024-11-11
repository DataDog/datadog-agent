// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog is a wrapper that loads workloadmeta collectors, while having less
// than the full set. Currently only used by the dogstatsd binary, this catalog does
// not include the process-collector due to its increased dependency set.
package catalog

import (
	"go.uber.org/fx"
)

// GetCatalog returns the set of FX options to populate the catalog
func GetCatalog() fx.Option {
	options := getCollectorOptions()

	// remove nil options
	opts := make([]fx.Option, 0, len(options))
	for _, item := range options {
		if item != nil {
			opts = append(opts, item)
		}
	}
	return fx.Options(opts...)
}
