module github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common

go 1.25.6

replace github.com/DataDog/datadog-agent => ../../../../..

require (
	github.com/DataDog/datadog-agent v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/config/setup v0.76.0-devel
	github.com/DataDog/datadog-agent/pkg/util/cache v0.75.2
	github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace v0.77.0-devel
	github.com/DataDog/datadog-agent/pkg/util/log v0.76.0-devel
	github.com/DataDog/datadog-agent/pkg/util/prometheus v0.75.2
	github.com/DataDog/datadog-agent/pkg/util/system v0.76.0-devel
	github.com/stretchr/testify v1.11.1
	k8s.io/api v0.35.0
	k8s.io/apimachinery v0.35.0
	k8s.io/client-go v0.35.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/secrets/def v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/create v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/helper v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/structure v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/viperconfig v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/fips v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/template v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.76.0-devel // indirect
	github.com/DataDog/viper v1.15.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.1 // indirect
	github.com/go-openapi/jsonreference v0.21.3 // indirect
	github.com/go-openapi/swag v0.25.4 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.4 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/fileutils v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/mangling v0.25.4 // indirect
	github.com/go-openapi/swag/netutils v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853 // indirect
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	github.com/mdlayher/vsock v1.2.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/prometheus v0.309.2-0.20260113170727-c7bc56cf6c8f // indirect
	github.com/shirou/gopsutil/v4 v4.25.12 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/term v0.39.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250910181357-589584f1c912 // indirect
	k8s.io/utils v0.0.0-20251002143259-bc988d571ff4 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)
