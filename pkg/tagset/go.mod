module github.com/DataDog/datadog-agent/pkg/tagset

go 1.22.0

replace github.com/DataDog/datadog-agent/pkg/util/sort => ../util/sort/

require (
	github.com/DataDog/datadog-agent/pkg/util/sort v0.56.0-rc.3
	github.com/stretchr/testify v1.10.0
	github.com/twmb/murmur3 v1.1.8
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
