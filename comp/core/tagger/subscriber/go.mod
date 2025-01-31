module github.com/DataDog/datadog-agent/comp/core/tagger/subscriber

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry => ../telemetry
	github.com/DataDog/datadog-agent/pkg/util/option => ../../../../pkg/util/option/
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.59.0
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.60.1
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.60.1
	github.com/DataDog/datadog-agent/pkg/util/log v0.60.1
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.59.0 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.60.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/option v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.60.1 // indirect
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
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/fx v1.23.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	google.golang.org/protobuf v1.36.3 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/comp/api/api/def => ../../../api/api/def

replace github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../flare/builder

replace github.com/DataDog/datadog-agent/comp/core/flare/types => ../../flare/types

replace github.com/DataDog/datadog-agent/comp/core/secrets => ../../secrets

replace github.com/DataDog/datadog-agent/comp/core/tagger/types => ../types

replace github.com/DataDog/datadog-agent/comp/core/tagger/utils => ../utils

replace github.com/DataDog/datadog-agent/comp/core/telemetry => ../../telemetry

replace github.com/DataDog/datadog-agent/comp/def => ../../../def

replace github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults

replace github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env

replace github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model

replace github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../../../pkg/config/nodetreemodel

replace github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup

replace github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../../../pkg/config/teeconfig

replace github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable

replace github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem

replace github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil

replace github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate

replace github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log

replace github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber

replace github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system

replace github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket

replace github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil

replace github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil

replace github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version
