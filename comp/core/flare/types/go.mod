module github.com/DataDog/datadog-agent/comp/core/flare/types

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../builder
	github.com/DataDog/datadog-agent/comp/def => ../../../def
)

require (
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.56.0-rc.3
	go.uber.org/fx v1.18.2
)

require (
	github.com/DataDog/datadog-agent/comp/def v0.56.0-rc.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
