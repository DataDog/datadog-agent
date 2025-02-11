module github.com/DataDog/datadog-agent/comp/core/log/mock

go 1.23.5

replace (
	github.com/DataDog/datadog-agent/comp/core/config => ../../config
	github.com/DataDog/datadog-agent/comp/core/log/def => ../def
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../secrets
	github.com/DataDog/datadog-agent/comp/def => ../../../def/
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/mock => ../../../../pkg/config/mock
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../../../pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../../../pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/config/viperconfig => ../../../../pkg/config/viperconfig
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/log/setup => ../../../../pkg/util/log/setup
	github.com/DataDog/datadog-agent/pkg/util/option => ../../../../pkg/util/option
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil

)

require (
	github.com/DataDog/datadog-agent/comp/core/log/def v0.64.0-devel
	github.com/DataDog/datadog-agent/pkg/util/log v0.62.2
	github.com/DataDog/datadog-agent/pkg/util/log/setup v0.64.0-devel
)

require (
	github.com/DataDog/datadog-agent/pkg/config/model v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.60.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/viperconfig v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.62.2 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.62.2 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/config/structure => ../../../../pkg/config/structure

replace github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version
