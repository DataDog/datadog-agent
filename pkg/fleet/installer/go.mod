module github.com/DataDog/datadog-agent/pkg/fleet/installer

go 1.25.0

require (
	cloud.google.com/go/compute/metadata v0.9.0
	github.com/DataDog/datadog-agent/pkg/template v0.73.2
	github.com/DataDog/datadog-agent/pkg/util/log v0.73.2
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.68.3
	github.com/DataDog/datadog-agent/pkg/version v0.73.2
	github.com/Microsoft/go-winio v0.6.2
	github.com/cenkalti/backoff/v5 v5.0.3
	github.com/fatih/color v1.18.0
	github.com/google/go-containerregistry v0.20.7
	github.com/google/uuid v1.6.0
	github.com/shirou/gopsutil/v4 v4.26.1
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.11.1
	go.etcd.io/bbolt v1.4.3
	go.uber.org/atomic v1.11.0
	go.uber.org/multierr v1.11.0
	golang.org/x/net v0.50.0
	golang.org/x/sys v0.41.0
	golang.org/x/text v0.34.0
	gopkg.in/evanphx/json-patch.v4 v4.12.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.73.2 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.18.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/cli v29.0.3+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.9.3 // indirect
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.18.3 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/vbatts/tar-split v0.12.2 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
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

replace github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../config/teeconfig

replace github.com/DataDog/datadog-agent/pkg/config/viperconfig => ../../config/viperconfig

replace github.com/DataDog/datadog-agent/pkg/fips => ../../fips

replace github.com/DataDog/datadog-agent/pkg/telemetry => ../../telemetry

replace github.com/DataDog/datadog-agent/pkg/util/executable => ../../util/executable

replace github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../util/filesystem

replace github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../util/fxutil

replace github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../util/hostname/validate

replace github.com/DataDog/datadog-agent/pkg/util/option => ../../util/option

replace github.com/DataDog/datadog-agent/pkg/util/pointer => ../../util/pointer

replace github.com/DataDog/datadog-agent/pkg/util/system => ../../util/system

replace github.com/DataDog/datadog-agent/pkg/util/testutil => ../../util/testutil

replace github.com/DataDog/datadog-agent/pkg/trace => ../../trace
