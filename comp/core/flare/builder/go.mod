module github.com/DataDog/datadog-agent/comp/core/flare/builder

go 1.22.0

replace github.com/DataDog/datadog-agent/comp/def => ../../../def

require github.com/DataDog/datadog-agent/comp/def v0.57.0-rc.1

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
