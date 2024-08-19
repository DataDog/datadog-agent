module github.com/DataDog/datadog-agent/pkg/util/fxutil

go 1.22.0

require (
	github.com/DataDog/datadog-agent/comp/def v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/optional v0.55.0
	github.com/spf13/cobra v1.7.0
	github.com/stretchr/testify v1.9.0
	go.uber.org/fx v1.18.2
)

replace (
	github.com/DataDog/datadog-agent/comp/def => ../../../comp/def
	github.com/DataDog/datadog-agent/pkg/util/optional => ../optional

)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/sys v0.23.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
