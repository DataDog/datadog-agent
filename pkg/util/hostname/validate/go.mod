module github.com/DataDog/datadog-agent/pkg/util/hostname/validate

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../../log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../scrubber/
)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0-rc.3
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
