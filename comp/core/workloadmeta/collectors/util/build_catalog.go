// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package util contains utility functions for workload metadata collectors
package util

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/wmcatalog/def"
)

func BuildCatalog(cfg config.Component, constructors ...func(config.Component) (wmcatalog.Collector, error)) []wmcatalog.Collector {
	results := []wmcatalog.Collector{}
	for _, ctor := range constructors {
		item, _ := ctor(cfg)
		if item != nil {
			results = append(results, item)
		}
	}
	return results
}
