module github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface

go 1.21.8

replace github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil

require (
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.53.0-rc.2
	github.com/stretchr/testify v1.9.0
	go.uber.org/fx v1.18.2
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/sys v0.0.0-20220412211240-33da011f77ad // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
