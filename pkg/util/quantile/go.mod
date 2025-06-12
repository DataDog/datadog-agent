module github.com/DataDog/datadog-agent/pkg/util/quantile

go 1.23.0

require (
	github.com/DataDog/datadog-agent/pkg/util/quantile/sketchtest v0.67.0-rc.9
	github.com/DataDog/sketches-go v1.4.7
	github.com/dustin/go-humanize v1.0.1
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/util/quantile/sketchtest => ../../../pkg/util/quantile/sketchtest

retract v0.4.0 // see #107
