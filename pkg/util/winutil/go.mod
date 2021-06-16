module github.com/DataDog/datadog-agent/pkg/util/winutil

go 1.15

replace github.com/DataDog/datadog-agent/pkg/util/log => ../log

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.29.0-rc.6
	golang.org/x/sys v0.0.0-20200930185726-fdedc70b468f
)
