# procmatch

Procmatch is a library that provides a way to extract datadog-integrations from command lines by using a catalog.

## Getting Started

### Installing

To install the package: `go get -u github.com/DataDog/datadog-agent/pkg/procmatch`

Usage:

```golang
package main

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/procmatch"
)

func main() {
	cmd := "java org.elasticsearch.bootstrap.elasticsearch"
	fmt.Printf("Integration: %s\n", procmatch.Match(cmd))
	// Outputs:
	// Integration: elastic
}
```

The default catalog is available [here](/default_catalog.go)

## Generate a new catalog

The catalog is generated [here](/gen/generate_catalog.go), you will first need to clone the [datadog integration-core repo](https://github.com/DataDog/integrations-core).

Then you can run `INTEGRATIONS_CORE_DIR=<path_to_integration_repo go generate` inside _procmatch_ directory.

The generation script reads process signatures from the `manifest.json` files.

## Using an other catalog

You can also use your own catalog

```golang
m, err := NewGraphMatcher(myCustomCatalog)
// Or
m := NewContainsMatcher(myCustomCatalog)
```

## Running the tests

To run the tests simply do: `go test ./...`

There are also benchmarks included, to run them you can do: `go test -bench=.`
