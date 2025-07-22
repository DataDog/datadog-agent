module github.com/DataDog/datadog-agent/pkg/fleet/installer

go 1.24.0

require (
	cloud.google.com/go/compute/metadata v0.7.0
	github.com/DataDog/datadog-agent/pkg/template v0.65.1
	github.com/DataDog/datadog-agent/pkg/util/log v0.64.0-devel.0.20250129111638-01c8fb06949e
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.64.3
	github.com/DataDog/datadog-agent/pkg/version v0.64.3
	github.com/Microsoft/go-winio v0.6.2
	github.com/cenkalti/backoff/v5 v5.0.2
	github.com/fatih/color v1.18.0
	github.com/google/go-containerregistry v0.20.3
	github.com/google/uuid v1.6.0
	github.com/shirou/gopsutil/v4 v4.25.5
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.10.0
	go.etcd.io/bbolt v1.4.0
	go.uber.org/atomic v1.11.0
	go.uber.org/multierr v1.11.0
	golang.org/x/net v0.41.0
	golang.org/x/sys v0.33.0
	golang.org/x/text v0.26.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.64.3 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.16.3 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/cli v27.5.0+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.2 // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20240909124753-873cd0166683 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.9.0 // indirect
	github.com/vbatts/tar-split v0.11.6 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.34.0 // indirect
	gotest.tools/v3 v3.5.2 // indirect
)

replace (
	github.com/DataDog/datadog-agent/pkg/template => ../../../pkg/template
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/version => ../../../pkg/version
)

replace github.com/DataDog/datadog-agent/pkg/util/winutil => ../../util/winutil
