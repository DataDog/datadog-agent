module github.com/DataDog/datadog-agent/comp/core/flare/types

go 1.21.0

replace (
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../builder
	github.com/DataDog/datadog-agent/comp/def => ../../../def
)

require (
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.55.0-rc.5
	go.uber.org/fx v1.18.2
)

require (
	github.com/DataDog/datadog-agent/comp/def v0.55.0-rc.5 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
)
