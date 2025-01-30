module github.com/DataDog/datadog-agent/comp/core/tagger/telemetry

go 1.22.0

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.59.0
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.60.1
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.59.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_golang v1.20.5 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	golang.org/x/sys v0.29.0 // indirect
	google.golang.org/protobuf v1.36.3 // indirect
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
