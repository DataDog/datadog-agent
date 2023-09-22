module github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types

go 1.20

replace github.com/DataDog/datadog-agent/pkg/util/log => ../../../util/log/

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.47.1
	github.com/stretchr/testify v1.8.4
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.48.0-rc.2 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
