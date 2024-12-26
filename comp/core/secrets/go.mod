module github.com/DataDog/datadog-agent/comp/core/secrets

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../api/api/def
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../flare/types
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../def
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../pkg/util/winutil

)

require (
	github.com/DataDog/datadog-agent/comp/api/api/def v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.56.0-rc.3
	github.com/benbjohnson/clock v1.3.5
	github.com/stretchr/testify v1.10.0
	go.uber.org/fx v1.23.0
	golang.org/x/exp v0.0.0-20241210194714-1829a127f884
	golang.org/x/sys v0.28.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.55.0 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.59.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.20.5 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.60.1 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	google.golang.org/protobuf v1.35.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../../pkg/version
