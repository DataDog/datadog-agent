module github.com/DataDog/datadog-agent/pkg/util/sds

go 1.25.9

replace github.com/DataDog/datadog-agent/pkg/version => ../../../pkg/version

replace github.com/DataDog/datadog-agent/pkg/template => ../../../pkg/template

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber

replace github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.0.0-00010101000000-000000000000
	github.com/DataDog/dd-sensitive-data-scanner/sds-go/go v0.0.0-20260601185219-3c48e9fa5604
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/DataDog/datadog-agent/pkg/template v0.64.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.64.1 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.62.3 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/time v0.15.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
