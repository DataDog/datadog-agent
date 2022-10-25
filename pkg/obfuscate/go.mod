module github.com/DataDog/datadog-agent/pkg/obfuscate

go 1.12

require (
	github.com/DataDog/datadog-go/v5 v5.1.1
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/dgraph-io/ristretto v0.1.1
	github.com/stretchr/testify v1.8.1
	go.uber.org/atomic v1.10.0
)

replace golang.org/x/net v0.0.0-20220722155237-a158d28d115b => golang.org/x/net v0.1.0
