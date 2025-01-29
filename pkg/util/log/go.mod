module github.com/DataDog/datadog-agent/pkg/util/log

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/util/compression => ../compression
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber
	github.com/cihub/seelog => github.com/cihub/seelog v0.0.0-20151216151435-d2c6e5aa9fbf // v2.6
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.63.0-rc.2
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/stretchr/testify v1.10.0
	go.uber.org/atomic v1.11.0
	go.uber.org/zap v1.27.0
)

require (
	github.com/DataDog/datadog-agent/pkg/version v0.63.0-rc.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
