module github.com/DataDog/datadog-agent/pkg/util/winutil

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber/
)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.63.0-rc.2
	github.com/fsnotify/fsnotify v1.8.0
	github.com/stretchr/testify v1.10.0
	go.uber.org/atomic v1.11.0
	golang.org/x/sys v0.29.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.63.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.63.0-rc.2 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
