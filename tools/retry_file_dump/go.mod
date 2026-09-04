// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

module github.com/DataDog/datadog-agent/tools/retry_file_dump

go 1.26.0

require (
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.84.0-rc.1
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/DataDog/datadog-agent/comp/core/config v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/delegatedauth v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/def v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets/def v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/basic v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/create v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/helper v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/mock v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup/constants v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/structure v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/fips v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/template v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/option v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.84.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.84.0-rc.1 // indirect
	github.com/DataDog/go-acl v1.0.1 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/ebitengine/purego v0.10.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gofrs/flock v0.13.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20260802145828-341c2f0c90b5 // indirect
	github.com/mdlayher/socket v0.6.1 // indirect
	github.com/mdlayher/vsock v1.3.0 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/power-devops/perfstat v0.0.0-20260805114148-88456608a4f6 // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/shirou/gopsutil/v4 v4.26.7 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.19.0 // indirect
	go.uber.org/fx v1.24.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

replace github.com/DataDog/datadog-agent/comp/core/config => ../../comp/core/config

replace github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def => ../../comp/core/configstreamconsumer/def

replace github.com/DataDog/datadog-agent/comp/core/delegatedauth => ../../comp/core/delegatedauth

replace github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../comp/core/flare/builder

replace github.com/DataDog/datadog-agent/comp/core/flare/types => ../../comp/core/flare/types

replace github.com/DataDog/datadog-agent/comp/core/log/def => ../../comp/core/log/def

replace github.com/DataDog/datadog-agent/comp/core/log/mock => ../../comp/core/log/mock

replace github.com/DataDog/datadog-agent/comp/core/secrets/def => ../../comp/core/secrets/def

replace github.com/DataDog/datadog-agent/comp/core/secrets/mock => ../../comp/core/secrets/mock

replace github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl => ../../comp/core/secrets/noop-impl

replace github.com/DataDog/datadog-agent/comp/core/secrets/utils => ../../comp/core/secrets/utils

replace github.com/DataDog/datadog-agent/comp/core/telemetry => ../../comp/core/telemetry

replace github.com/DataDog/datadog-agent/comp/def => ../../comp/def

replace github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ../../comp/forwarder/defaultforwarder

replace github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../pkg/collector/check/defaults

replace github.com/DataDog/datadog-agent/pkg/config/basic => ../../pkg/config/basic

replace github.com/DataDog/datadog-agent/pkg/config/create => ../../pkg/config/create

replace github.com/DataDog/datadog-agent/pkg/config/env => ../../pkg/config/env

replace github.com/DataDog/datadog-agent/pkg/config/helper => ../../pkg/config/helper

replace github.com/DataDog/datadog-agent/pkg/config/mock => ../../pkg/config/mock

replace github.com/DataDog/datadog-agent/pkg/config/model => ../../pkg/config/model

replace github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../pkg/config/nodetreemodel

replace github.com/DataDog/datadog-agent/pkg/config/setup => ../../pkg/config/setup

replace github.com/DataDog/datadog-agent/pkg/config/setup/constants => ../../pkg/config/setup/constants

replace github.com/DataDog/datadog-agent/pkg/config/structure => ../../pkg/config/structure

replace github.com/DataDog/datadog-agent/pkg/config/utils => ../../pkg/config/utils

replace github.com/DataDog/datadog-agent/pkg/fips => ../../pkg/fips

replace github.com/DataDog/datadog-agent/pkg/template => ../../pkg/template

replace github.com/DataDog/datadog-agent/pkg/util/defaultpaths => ../../pkg/util/defaultpaths

replace github.com/DataDog/datadog-agent/pkg/util/executable => ../../pkg/util/executable

replace github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../pkg/util/filesystem

replace github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../pkg/util/fxutil

replace github.com/DataDog/datadog-agent/pkg/util/log => ../../pkg/util/log

replace github.com/DataDog/datadog-agent/pkg/util/option => ../../pkg/util/option

replace github.com/DataDog/datadog-agent/pkg/util/pointer => ../../pkg/util/pointer

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../pkg/util/scrubber

replace github.com/DataDog/datadog-agent/pkg/util/system => ../../pkg/util/system

replace github.com/DataDog/datadog-agent/pkg/util/testutil => ../../pkg/util/testutil

replace github.com/DataDog/datadog-agent/pkg/util/winutil => ../../pkg/util/winutil

replace github.com/DataDog/datadog-agent/pkg/version => ../../pkg/version
