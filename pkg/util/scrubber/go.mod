module github.com/DataDog/datadog-agent/pkg/util/scrubber

go 1.22.0

require (
	github.com/DataDog/datadog-agent/pkg/version v0.62.0-rc.7
	github.com/stretchr/testify v1.10.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
