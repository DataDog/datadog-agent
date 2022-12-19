module github.com/DataDog/datadog-agent/pkg/util/cgroups

go 1.18

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber
)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.42.0-rc.3
	github.com/containerd/cgroups v1.0.4
	github.com/google/go-cmp v0.5.8
	github.com/karrick/godirwalk v1.17.0
	github.com/stretchr/testify v1.8.1
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.42.0-rc.3 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/godbus/dbus/v5 v5.0.4 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/opencontainers/runtime-spec v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
