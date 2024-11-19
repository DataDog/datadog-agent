module github.com/DataDog/datadog-agent/pkg/util/grpc

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/proto => ../../proto
	github.com/DataDog/datadog-agent/pkg/util/log => ../log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber
)

require (
	github.com/DataDog/datadog-agent/pkg/api v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/proto v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0-rc.3
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/net v0.31.0
	google.golang.org/grpc v1.59.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/secrets v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.56.0-rc.3 // indirect
	github.com/DataDog/viper v1.13.5 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shirou/gopsutil/v3 v3.23.12 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20240909161429-701f63a606c0 // indirect
	golang.org/x/mod v0.21.0 // indirect
	golang.org/x/sync v0.9.0 // indirect
	golang.org/x/sys v0.27.0 // indirect
	golang.org/x/text v0.20.0 // indirect
	golang.org/x/tools v0.25.0 // indirect
	google.golang.org/genproto v0.0.0-20231106174013-bbf56f31fb17 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20231106174013-bbf56f31fb17 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231120223509-83a465c0220f // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
