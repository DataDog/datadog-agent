module github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface

go 1.21

replace github.com/DataDog/datadog-agent/comp/core/status => ../../../../comp/core/status

require github.com/DataDog/datadog-agent/comp/core/status v0.0.0-00010101000000-000000000000

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	go.uber.org/atomic v1.6.0 // indirect
	go.uber.org/dig v1.15.0 // indirect
	go.uber.org/fx v1.18.2 // indirect
	go.uber.org/multierr v1.5.0 // indirect
	go.uber.org/zap v1.16.0 // indirect
	golang.org/x/sys v0.14.0 // indirect
	golang.org/x/text v0.3.0 // indirect
)
