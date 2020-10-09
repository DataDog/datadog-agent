module github.com/DataDog/datadog-agent/pkg/util/winutil

replace (
    github.com/DataDog/datadog-agent/pkg/util/winutil => ../winutil
)

require (
    "github.com/DataDog/datadog-agent/pkg/util/log" v0.0.0-20201009091026-5e3e70109784
)