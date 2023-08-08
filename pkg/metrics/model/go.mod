module github.com/DataDog/datadog-agent/pkg/metrics/model

go 1.20

replace (
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ../../aggregator/ckey/
	github.com/DataDog/datadog-agent/pkg/tagset => ../../tagset/
	github.com/DataDog/datadog-agent/pkg/util/util_sort => ../../util/util_sort/
)

require (
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/tagset v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.8.4
)

require (
	github.com/DataDog/datadog-agent/pkg/util/util_sort v0.0.0-00010101000000-000000000000 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
