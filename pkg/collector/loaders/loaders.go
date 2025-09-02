// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package loaders contains the logic to sort the loaders
package loaders

import (
	"sort"
	"sync"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// LoaderFactory helps to defer actual instantiation of Check Loaders,
// mostly helpful with code involving calls to cgo (for example, the Python
// interpreter might not be initialized when `init`ing a package)
type LoaderFactory func(sender.SenderManager, option.Option[integrations.Component], tagger.Component) (check.Loader, int, error)

var factoryCatalog = []LoaderFactory{}
var loaderCatalog = []check.Loader{}
var once sync.Once

// RegisterLoader adds a loader to the loaderCatalog
func RegisterLoader(factory LoaderFactory) {
	factoryCatalog = append(factoryCatalog, factory)
}

// LoaderCatalog returns the loaders sorted by desired sequence order
func LoaderCatalog(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component) []check.Loader {
	// the catalog is supposed to be built only once, don't see a clear
	// use case to add Loaders at runtime
	once.Do(func() {
		loaders := make(map[int][]check.Loader, len(factoryCatalog))
		for _, factory := range factoryCatalog {
			loader, order, err := factory(senderManager, logReceiver, tagger)
			if err != nil {
				log.Infof("Failed to instantiate %s: %v", loader, err)
				continue
			}
			loaders[order] = append(loaders[order], loader)
		}

		// get the desired sequences, sorted least to greatest
		var keys []int
		for k := range loaders {
			keys = append(keys, k)
		}
		sort.Ints(keys)

		// use the desired sequences to access the catalog and build
		// the final slice of loaders
		for _, k := range keys {
			loaderCatalog = append(loaderCatalog, loaders[k]...)
		}

	})

	return loaderCatalog
}
