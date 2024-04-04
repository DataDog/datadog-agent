module github.com/DataDog/datadog-agent/pkg/config/remote

go 1.21.8

replace (
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../comp/core/telemetry
	github.com/DataDog/datadog-agent/pkg/config/model => ../model
	github.com/DataDog/datadog-agent/pkg/proto => ../../proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../telemetry
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../../util/backoff
	github.com/DataDog/datadog-agent/pkg/util/cache => ../../util/cache
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/grpc => ../../util/grpc
	github.com/DataDog/datadog-agent/pkg/util/http => ../../util/http
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/uuid => ../../util/uuid
)

require (
	github.com/DataDog/datadog-agent/pkg/config/model v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/proto v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/grpc v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/http v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/log v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/uuid v0.53.0-rc.2
	github.com/Masterminds/semver v1.5.0
	github.com/benbjohnson/clock v1.3.0
	github.com/pkg/errors v0.9.1
	github.com/secure-systems-lab/go-securesystemslib v0.7.0
	github.com/stretchr/testify v1.9.0
	go.etcd.io/bbolt v1.3.7
	go.uber.org/atomic v1.11.0
	google.golang.org/protobuf v1.31.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/cache v0.53.0-rc.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.1 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	golang.org/x/exp v0.0.0-20240222234643-814bf88cf225 // indirect
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.53.0-rc.2 // indirect
	github.com/DataDog/go-tuf v1.1.0-0.5.2
	github.com/DataDog/viper v1.13.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.4.7 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/cast v1.3.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/grpc v1.59.0
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
