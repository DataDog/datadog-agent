module github.com/DataDog/datadog-agent/comp/core/log/mock

go 1.21.0

replace (
	github.com/DataDog/datadog-agent/comp/core/config => ../../config
	github.com/DataDog/datadog-agent/comp/core/log/def => ../def
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../secrets
	github.com/DataDog/datadog-agent/comp/def => ../../../def/
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/logs => ../../../../pkg/config/logs/
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
)

require (
	github.com/DataDog/datadog-agent/comp/core/log/def v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0-rc.2
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
