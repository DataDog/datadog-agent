module github.com/DataDog/datadog-agent/pkg/gohai

// we don't want to just use the agent's go version because gohai might be used outside of it
// eg. opentelemetry
go 1.16

require (
	github.com/cihub/seelog v0.0.0-20151216151435-d2c6e5aa9fbf
	github.com/shirou/gopsutil/v3 v3.22.12
	github.com/stretchr/testify v1.8.2
	golang.org/x/sys v0.3.0
)
