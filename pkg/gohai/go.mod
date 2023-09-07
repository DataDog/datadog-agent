module github.com/DataDog/datadog-agent/pkg/gohai

// we don't want to just use the agent's go version because gohai might be used outside of it
// eg. opentelemetry
go 1.20

require (
	github.com/cihub/seelog v0.0.0-20151216151435-d2c6e5aa9fbf
	github.com/moby/sys/mountinfo v0.6.2
	github.com/shirou/gopsutil/v3 v3.23.8
	github.com/stretchr/testify v1.8.4
	golang.org/x/sys v0.11.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
