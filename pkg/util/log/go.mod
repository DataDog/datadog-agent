module github.com/DataDog/datadog-agent/pkg/util/log

go 1.18

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.45.0-rc.3
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/stretchr/testify v1.8.1
	go.uber.org/zap v1.22.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
