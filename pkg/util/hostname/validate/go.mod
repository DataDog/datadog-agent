module github.com/DataDog/datadog-agent/pkg/util/hostname/validate

go 1.21.8

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../../log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../scrubber/
)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.53.0-rc.2
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.53.0-rc.2 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
