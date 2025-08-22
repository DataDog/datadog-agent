module github.com/DataDog/datadog-agent/pkg/network/driver

go 1.24.0

replace github.com/DataDog/datadog-agent/pkg/util/option => ../../../pkg/util/option

replace github.com/DataDog/datadog-agent/pkg/telemetry => ../../../pkg/telemetry

replace github.com/DataDog/datadog-agent/comp/def => ../../../comp/def

replace github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log

replace github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../pkg/util/winutil

replace github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../pkg/util/fxutil

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber

replace github.com/DataDog/datadog-agent/pkg/version => ../../../pkg/version

replace github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../comp/core/telemetry

require (
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.61.0
	github.com/DataDog/datadog-agent/pkg/telemetry v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/log v0.64.0-devel
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.10.0
	go.uber.org/atomic v1.11.0
	golang.org/x/sys v0.35.0
)

require (
	github.com/DataDog/datadog-agent/comp/def v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/option v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.62.3 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.62.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.22.0 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.65.0 // indirect
	github.com/prometheus/procfs v0.17.0 // indirect
	github.com/spf13/cobra v1.9.1 // indirect
	github.com/spf13/pflag v1.0.7 // indirect
	go.uber.org/dig v1.19.0 // indirect
	go.uber.org/fx v1.24.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
