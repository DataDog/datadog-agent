module github.com/DataDog/datadog-agent/pkg/fleet/installer

go 1.23.0

require (
	cloud.google.com/go/compute/metadata v0.6.0
	github.com/DataDog/datadog-agent/pkg/util/log v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/version v0.59.1
	github.com/DataDog/gopsutil v1.2.2
	github.com/Microsoft/go-winio v0.6.2
	github.com/google/go-containerregistry v0.20.3
	github.com/google/uuid v1.6.0
	github.com/shirou/gopsutil/v4 v4.24.12
	github.com/stretchr/testify v1.10.0
	go.etcd.io/bbolt v1.3.11
	go.uber.org/atomic v1.11.0
	go.uber.org/multierr v1.11.0
	golang.org/x/net v0.34.0
	golang.org/x/sys v0.30.0
	golang.org/x/text v0.21.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/StackExchange/wmi v0.0.0-20181212234831-e0a55b97c705 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.16.3 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/cli v27.5.0+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.2 // indirect
	github.com/ebitengine/purego v0.8.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/vbatts/tar-split v0.11.6 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/oauth2 v0.25.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
)

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/version => ../../../pkg/version
)
