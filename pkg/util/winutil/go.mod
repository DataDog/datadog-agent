module github.com/DataDog/datadog-agent/pkg/util/winutil

go 1.16

replace github.com/DataDog/datadog-agent/pkg/util/log => ../log

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.33.0-rc.4
	github.com/stretchr/testify v1.7.0
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007
)
