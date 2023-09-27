module github.com/DataDog/datadog-agent/comp/forwarder

go 1.20
replace (
    github.com/DataDog/datadog-agent/cmd/agent/common/path => ../../../cmd/agent/common/path
    github.com/DataDog/datadog-agent/comp/core/config => ../config/
    github.com/DataDog/datadog-agent/comp/core/telemetry => ../../core/telemetry
    github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types => ../../../pkg/autodiscovery/common/types
    github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../pkg/collector/check/defaults
    github.com/DataDog/datadog-agent/pkg/conf => ../../../pkg/conf
    github.com/DataDog/datadog-agent/pkg/config/configsetup => ../../../pkg/config/configsetup
    github.com/DataDog/datadog-agent/pkg/config/utils/endpoints => ../../../pkg/config/load
    github.com/DataDog/datadog-agent/pkg/config/load => ../../../pkg/config/load
    github.com/DataDog/datadog-agent/pkg/config/logsetup => ../../pkg/config/logsetup/
    github.com/DataDog/datadog-agent/pkg/secrets => ../../pkg/secrets
    github.com/DataDog/datadog-agent/pkg/telemetry => ../../pkg/telemetry
    github.com/DataDog/datadog-agent/pkg/version => ../../pkg/version
 github.com/DataDog/datadog-agent/pkg/util/http => ../../pkg/util/http/
 github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../pkg/util/scrubber/
github.com/DataDog/datadog-agent/pkg/orchestrator/model => ../../pkg/orchestrator/model/
    github.com/DataDog/datadog-agent/pkg/status/health => ../../../pkg/status/health
    github.com/DataDog/datadog-agent/pkg/util/executable => ../../../pkg/util/executable
    github.com/DataDog/datadog-agent/pkg/util/common => ../../../pkg/util/common
    github.com/DataDog/datadog-agent/pkg/util/backoff => ../../pkg/util/backoff/
    github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../pkg/util/filesystem/
    github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../pkg/util/fxutil
    github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../pkg/util/system/socket
    github.com/DataDog/datadog-agent/comp/core/log => ../core/log/
)

