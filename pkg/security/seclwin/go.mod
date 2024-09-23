module github.com/DataDog/datadog-agent/pkg/security/seclwin

go 1.22.0

replace github.com/DataDog/datadog-agent/pkg/security/secl => ../secl

require (
	github.com/DataDog/datadog-agent/pkg/security/secl v0.57.2-rc.2
	modernc.org/mathutil v1.6.0
)

require (
	github.com/alecthomas/participle v0.7.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
)
