module github.com/DataDog/datadog-agent/pkg/tagset

go 1.21.8

replace github.com/DataDog/datadog-agent/pkg/util/sort => ../util/sort/

require (
	github.com/DataDog/datadog-agent/pkg/util/sort v0.53.0-rc.2
	github.com/stretchr/testify v1.9.0
	github.com/twmb/murmur3 v1.1.8
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
