module github.com/DataDog/datadog-agent/pkg/fleet/installer

go 1.26.0

require (
	cloud.google.com/go/compute/metadata v0.9.0
	github.com/DataDog/datadog-agent/pkg/fips v0.83.0-devel.0.20260729075015-99ed037f1c29
	github.com/DataDog/datadog-agent/pkg/template v0.73.2
	github.com/DataDog/datadog-agent/pkg/util/log v0.73.2
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.68.3
	github.com/DataDog/datadog-agent/pkg/version v0.73.2
	github.com/Microsoft/go-winio v0.6.2
	github.com/cenkalti/backoff/v7 v7.0.0
	github.com/evanphx/json-patch/v5 v5.9.11
	github.com/fatih/color v1.19.0
	github.com/google/go-containerregistry v0.21.9
	github.com/google/uuid v1.6.0
	github.com/itchyny/gojq v0.12.19
	github.com/shirou/gopsutil/v4 v4.26.7
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.12.1
	go.etcd.io/bbolt v1.5.0
	go.uber.org/atomic v1.11.0
	go.uber.org/multierr v1.11.0
	go.yaml.in/yaml/v2 v2.4.4
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/net v0.58.0
	golang.org/x/sys v0.47.0
	golang.org/x/text v0.41.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.73.2 // indirect
	github.com/docker/cli v29.6.2+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.9.4 // indirect
	github.com/ebitengine/purego v0.10.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20260802145828-341c2f0c90b5 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20260805114148-88456608a4f6 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/sirupsen/logrus v1.10.1 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gotest.tools/v3 v3.5.2 // indirect
)

replace (
	github.com/DataDog/datadog-agent/pkg/template => ../../../pkg/template
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/version => ../../../pkg/version
)

replace github.com/DataDog/datadog-agent/pkg/util/winutil => ../../util/winutil

replace github.com/DataDog/datadog-agent/comp/api/api/def => ../../../comp/api/api/def

replace github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../../comp/core/flare/builder

replace github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../comp/core/flare/types

replace github.com/DataDog/datadog-agent/comp/core/status => ../../../comp/core/status

replace github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../comp/core/telemetry

replace github.com/DataDog/datadog-agent/comp/def => ../../../comp/def

replace github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../collector/check/defaults

replace github.com/DataDog/datadog-agent/pkg/config/create => ../../config/create

replace github.com/DataDog/datadog-agent/pkg/config/env => ../../config/env

replace github.com/DataDog/datadog-agent/pkg/config/model => ../../config/model

replace github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../config/nodetreemodel

replace github.com/DataDog/datadog-agent/pkg/config/setup => ../../config/setup

replace github.com/DataDog/datadog-agent/pkg/config/structure => ../../config/structure

replace github.com/DataDog/datadog-agent/pkg/fips => ../../fips

replace github.com/DataDog/datadog-agent/pkg/util/executable => ../../util/executable

replace github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../util/filesystem

replace github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../util/fxutil

replace github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../util/hostname/validate

replace github.com/DataDog/datadog-agent/pkg/util/option => ../../util/option

replace github.com/DataDog/datadog-agent/pkg/util/pointer => ../../util/pointer

replace github.com/DataDog/datadog-agent/pkg/util/system => ../../util/system

replace github.com/DataDog/datadog-agent/pkg/util/testutil => ../../util/testutil

replace github.com/DataDog/datadog-agent/pkg/trace => ../../trace

replace github.com/DataDog/datadog-agent/pkg/credential => ../../credential
