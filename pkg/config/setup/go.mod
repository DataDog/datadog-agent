module github.com/DataDog/datadog-agent/pkg/config/setup

go 1.20

replace (

github.com/DataDog/datadog-agent/pkg/util/winutil => ../../util/winutil
github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log
github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../collector/check/defaults
github.com/DataDog/datadog-agent/pkg/config/model => ../model/
github.com/DataDog/datadog-agent/pkg/util/executable => ../../util/executable
github.com/DataDog/datadog-agent/pkg/config/env => ../env
github.com/DataDog/datadog-agent/pkg/secrets => ../../secrets
github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../util/hostname/validate
)
