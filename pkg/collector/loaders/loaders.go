// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package loaders

import (
	"sort"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LoaderFactory helps to defer actual instantiation of Check Loaders,
// mostly helpful with code involving calls to cgo (for example, the Python
// interpreter might not be initialized when `init`ing a package)
type LoaderFactory func() (check.Loader, error)

var factoryCatalog = make(map[int][]LoaderFactory)
var loaderCatalog = []check.Loader{}
var once sync.Once

// RegisterLoader adds a loader to the loaderCatalog
func RegisterLoader(order int, factory LoaderFactory) {
	factoryCatalog[order] = append(factoryCatalog[order], factory)
}

// LoaderCatalog returns the loaders sorted by desired sequence order
func LoaderCatalog() []check.Loader {
	// the catalog is supposed to be built only once, don't see a clear
	// use case to add Loaders at runtime
	once.Do(func() {
		// get the desired sequences, sorted least to greatest
		var keys []int
		for k := range factoryCatalog {
			keys = append(keys, k)
		}
		sort.Ints(keys)

		// use the desired sequences to access the catalog and build
		// the final slice of loaders
		for _, k := range keys {
			for _, factory := range factoryCatalog[k] {
				loader, err := factory()
				if err != nil {
					log.Infof("Failed to instantiate %s: %v", loader, err)
					continue
				}

				loaderCatalog = append(loaderCatalog, loader)
			}
		}

	})

	return loaderCatalog
}
