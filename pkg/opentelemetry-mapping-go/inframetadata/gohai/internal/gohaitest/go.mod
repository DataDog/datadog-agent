module github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/gohai/internal/gohaitest

go 1.23.0

require (
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata v0.67.0-rc.9
	github.com/DataDog/gohai v0.0.0-20230524154621-4316413895ee
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	golang.org/x/sys v0.32.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata => ../../../../../../pkg/opentelemetry-mapping-go/inframetadata
