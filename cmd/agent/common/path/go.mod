module github.com/DataDog/datadog-agent/cmd/agent/common/path

go 1.20

replace github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable

replace github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log

require (
	github.com/DataDog/datadog-agent/pkg/util/executable v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/log v0.49.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.36.1
	golang.org/x/sys v0.12.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.49.0-rc.2 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
