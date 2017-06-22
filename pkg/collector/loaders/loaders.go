package loaders

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// LoaderFactory a factory to check loaders
type LoaderFactory func() check.Loader

// LoaderCatalog keeps track of Go loaders by name
var LoaderCatalog = make(map[string]LoaderFactory)

// RegisterLoader adds a loader to the loaderCatalog
func RegisterLoader(name string, l LoaderFactory) {
	LoaderCatalog[name] = l
}
