module github.com/DataDog/datadog-agent/cmd/agent/common/path

go 1.21.9

replace (
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
)

require (
	github.com/DataDog/datadog-agent/pkg/util/executable v0.53.1-rc.1
	github.com/DataDog/datadog-agent/pkg/util/log v0.53.1-rc.1
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.53.1-rc.1
	golang.org/x/sys v0.14.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.53.1-rc.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)