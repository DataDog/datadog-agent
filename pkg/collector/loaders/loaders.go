package loaders

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// LoaderCatalog keeps track of Go loaders by name
var LoaderCatalog = make(map[string]check.Loader)

// RegisterLoader adds a loader to the loaderCatalog
func RegisterLoader(name string, l check.Loader) {
	LoaderCatalog[name] = l
}
