// Deprecated: This module will be removed in the 7.37 release cycle and integrated back in the main Datadog Agent module.
module github.com/DataDog/datadog-agent/pkg/util/winutil

go 1.17

replace github.com/DataDog/datadog-agent/pkg/util/log => ../log

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.35.0-rc.4
	github.com/stretchr/testify v1.7.1
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.35.0-rc.4 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)
