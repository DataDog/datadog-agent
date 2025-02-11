module github.com/DataDog/datadog-agent/comp/core/tagger/generic_store

go 1.22.0

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.59.0
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.59.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/comp/api/api/def => ../../../api/api/def

replace github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../flare/builder

replace github.com/DataDog/datadog-agent/comp/core/flare/types => ../../flare/types

replace github.com/DataDog/datadog-agent/comp/core/secrets => ../../secrets

replace github.com/DataDog/datadog-agent/comp/core/tagger/types => ../types

replace github.com/DataDog/datadog-agent/comp/core/tagger/utils => ../utils

replace github.com/DataDog/datadog-agent/comp/core/telemetry => ../../telemetry

replace github.com/DataDog/datadog-agent/comp/def => ../../../def

replace github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults

replace github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env

replace github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model

replace github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../../../pkg/config/nodetreemodel

replace github.com/DataDog/datadog-agent/pkg/config/viperconfig => ../../../../pkg/config/viperconfig

replace github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup

replace github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../../../pkg/config/teeconfig

replace github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable

replace github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem

replace github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil

replace github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate

replace github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log

replace github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber

replace github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system

replace github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket

replace github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil

replace github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
