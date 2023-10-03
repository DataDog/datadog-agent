module github.com/DataDog/datadog-agent/pkg/logs/message

go 1.20

replace (
	github.com/DataDog/datadog-agent/comp/core/config => ../../../comp/core/config
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../comp/core/telemetry
	github.com/DataDog/datadog-agent/comp/logs/agent/config => ../../../comp/logs/agent/config
	github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types => ../../autodiscovery/common/types
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/conf => ../../conf
	github.com/DataDog/datadog-agent/pkg/config/configsetup => ../../config/configsetup
	github.com/DataDog/datadog-agent/pkg/config/load => ../../config/load
	github.com/DataDog/datadog-agent/pkg/logs/internal/status => ../internal/status
	github.com/DataDog/datadog-agent/pkg/logs/sources => ../sources
	github.com/DataDog/datadog-agent/pkg/secrets => ../../secrets
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../telemetry
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../util/executable
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/stats_tracker => ../../util/stats_tracker
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../util/system/socket
	github.com/DataDog/datadog-agent/pkg/version => ../../version
)

require (
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/logs/sources v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.8.4
)

require (
	github.com/DataDog/datadog-agent/pkg/conf v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/internal/status v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.48.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.48.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/stats_tracker v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20220423185008-bf980b35cac4 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
