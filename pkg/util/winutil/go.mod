module github.com/DataDog/datadog-agent/pkg/util/winutil

replace (
    github.com/DataDog/datadog-agent/pkg/util/winutil => ../winutil
)

require (
    "github.com/DataDog/datadog-agent/pkg/util/log" v0.0.0
)