module github.com/DataDog/datadog-agent/comp/core/flare/types

go 1.22.0

replace github.com/DataDog/datadog-agent/comp/core/flare/builder => ../builder

require (
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.62.2-rc.1
	go.uber.org/fx v1.23.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
)
