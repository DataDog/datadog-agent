module github.com/DataDog/datadog-agent/comp/serializer/otelcompression

go 1.23.3

replace (
	github.com/DataDog/datadog-agent/comp/core/config => ../../core/config
	github.com/DataDog/datadog-agent/pkg/util/compression => ../../../pkg/util/compression
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths => ../../../pkg/util/defaultpaths
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../pkg/util/fxutil
)

require (
	github.com/DataDog/datadog-agent/pkg/util/compression v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.59.0
)

require (
	github.com/DataDog/datadog-agent/comp/def v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.59.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/fx v1.23.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
