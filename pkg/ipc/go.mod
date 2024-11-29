module github.com/DataDog/datadog-agent/pkg/ipc

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../comp/api/api/def
	github.com/DataDog/datadog-agent/comp/core/config => ../../comp/core/config
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../comp/core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../comp/core/flare/types
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../comp/core/secrets
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../comp/core/telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../comp/def
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config => ../config
	github.com/DataDog/datadog-agent/pkg/config/env => ../config/env
	github.com/DataDog/datadog-agent/pkg/config/mock => ../config/mock
	github.com/DataDog/datadog-agent/pkg/config/model => ../config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/setup => ../config/setup
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/config/utils => ../config/utils
	github.com/DataDog/datadog-agent/pkg/telemetry => ../telemetry
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths => ../../pkg/util/defaultpaths
	github.com/DataDog/datadog-agent/pkg/util/executable => ../util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system => ../util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../version
)

require (
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/log v0.57.1
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.57.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shirou/gopsutil/v3 v3.23.9 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.27.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
