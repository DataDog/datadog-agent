module github.com/DataDog/datadog-agent/pkg/security/seclwin

go 1.21

replace github.com/DataDog/datadog-agent/pkg/security/secl => ../secl

require (
	github.com/DataDog/datadog-agent/pkg/security/secl v0.54.0-rc.1
	github.com/hashicorp/golang-lru/v2 v2.0.7
)

require github.com/alecthomas/participle v0.7.1 // indirect
