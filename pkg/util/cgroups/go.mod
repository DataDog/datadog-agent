module github.com/DataDog/datadog-agent/pkg/util/cgroups

go 1.18

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../log
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber
)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.45.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.45.0-rc.3
	github.com/google/go-cmp v0.5.8
	github.com/karrick/godirwalk v1.17.0
	github.com/stretchr/testify v1.8.1
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.45.0-rc.3 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
