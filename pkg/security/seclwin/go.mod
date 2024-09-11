module github.com/DataDog/datadog-agent/pkg/security/seclwin

go 1.22.0

replace github.com/DataDog/datadog-agent/pkg/security/secl => ../secl

require (
	github.com/DataDog/datadog-agent/pkg/security/secl v0.58.0-rc.2
	modernc.org/mathutil v1.6.0
)

require (
	github.com/alecthomas/participle v0.7.1 // indirect
	github.com/jellydator/ttlcache/v3 v3.3.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sync v0.8.0 // indirect
)
