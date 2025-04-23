module github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata/gohai/internal/gohaitest

go 1.23.0

require (
	github.com/DataDog/gohai v0.0.0-20230524154621-4316413895ee
	github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata v0.27.0
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/cihub/seelog v0.0.0-20151216151435-d2c6e5aa9fbf // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata => ../../../
