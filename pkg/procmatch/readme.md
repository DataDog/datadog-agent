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
    matcher, err := procmatch.NewDefault()
    if err != nil {
        // Handle the error
    }

    cmd := "java org.elasticsearch.bootstrap.elasticsearch"
    fmt.Printf("Integration: %s\n", matcher.Match(cmd).Name)
    // Outputs:
    // Integration: elastic
}
```

The default catalog is available [here](/default_catalog.go)

## Adding a new integration's signature

To add a new integration signature, you will first have to add it inside its `manifest.json` file in the [datadog integration-core repo](https://github.com/DataDog/integrations-core) under the `process_signatures` field.

After that create a PR on the datadog integration-core repo to commit your change.

Then you can regenerate the default catalog to get the new signature here (see section below to regenerate the catalog)

For testing purposes you can also modify the generated file `/default_catalog.go`.

## Generate a new catalog

The catalog is generated [here](/gen/generate_catalog.go), you will first need to clone the [datadog integration-core repo](https://github.com/DataDog/integrations-core).

Then you can run `STACKSTATE_INTEGRATIONS_DIR=<path_to_integration_repo> go generate ./...` or `STACKSTATE_INTEGRATIONS_DIR=<path_to_integration_repo> go generate ./pkg/procmatch` if you're at the repo root.

The generation script reads process signatures from the `manifest.json` files.

## Adding tests for an integration

To add a new test for a given integration you can add an entry inside the test cases in `TestMatchIntegration` under `graph_matcher_test.go`, it will try to match an integration on your provided command line by using the default catalog.

## Running the tests

To run the tests simply do: `go test ./...`

There are also benchmarks included, to run them you can do: `go test -bench=.`
