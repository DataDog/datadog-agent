// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

module github.com/DataDog/datadog-agent/tools/retry_file_dump

go 1.26.0

require (
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.82.1
	google.golang.org/protobuf v1.36.12-0.20260116114154-8c4c4ae446ca
)

require (
	github.com/DataDog/datadog-agent/comp/core/config v0.82.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def v0.82.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/delegatedauth v0.82.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.82.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.82.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/def v0.82.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets/def v0.82.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl v0.82.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.82.1 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/basic v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/buildschema v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/create v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/helper v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/mock v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/structure v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/fips v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/template v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/option v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.82.1 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.82.1 // indirect
	github.com/DataDog/go-acl v1.0.1 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/ebitengine/purego v0.10.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20260330125221-c963978e514e // indirect
	github.com/mdlayher/socket v0.6.0 // indirect
	github.com/mdlayher/vsock v1.3.0 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/shirou/gopsutil/v4 v4.26.7 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.19.0 // indirect
	go.uber.org/fx v1.24.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
