module github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/quantile

go 1.23.0

require (
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/internal/sketchtest v0.0.0-00010101000000-000000000000
	github.com/DataDog/sketches-go v1.4.7
	github.com/dustin/go-humanize v1.0.1
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	google.golang.org/protobuf v1.36.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/internal/sketchtest => ../../../pkg/opentelemetry-mapping-go/internal/sketchtest

retract v0.4.0 // see #107
