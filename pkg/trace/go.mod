module github.com/DataDog/datadog-agent/pkg/trace

go 1.18

// NOTE: Prefer using simple `require` directives instead of using `replace` if possible.
// See https://github.com/DataDog/datadog-agent/blob/main/docs/dev/gomodreplace.md
// for more details.

// Internal deps fix version
replace github.com/docker/distribution => github.com/docker/distribution v2.8.1+incompatible

require (
	github.com/DataDog/datadog-agent v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.45.0-rc.3
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.45.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.45.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/log v0.45.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.45.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.45.0-rc.3
	github.com/DataDog/datadog-go/v5 v5.1.1
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.2.0
	github.com/DataDog/sketches-go v1.4.2
	github.com/Microsoft/go-winio v0.6.0
	github.com/davecgh/go-spew v1.1.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.3
	github.com/google/gofuzz v1.2.0
	github.com/google/uuid v1.3.0
	github.com/shirou/gopsutil/v3 v3.23.2
	github.com/stretchr/testify v1.8.2
	github.com/tinylib/msgp v1.1.6
	github.com/vmihailenco/msgpack/v4 v4.3.12
	go.opentelemetry.io/collector/pdata v1.0.0-rcv0011
	go.opentelemetry.io/collector/semconv v0.75.0
	go.uber.org/atomic v1.11.0
	golang.org/x/sys v0.8.0
	golang.org/x/time v0.3.0
	google.golang.org/grpc v1.54.0
	google.golang.org/protobuf v1.30.0
	k8s.io/apimachinery v0.25.5
)

require (
	github.com/CycloneDX/cyclonedx-go v0.7.0 // indirect
	github.com/DataDog/go-tuf v0.3.0--fix-localmeta-fork // indirect
	github.com/DataDog/gohai v0.0.0-20221116153829-5d479901d2e9 // indirect
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/DataDog/watermarkpodautoscaler v0.5.2 // indirect
	github.com/aws/aws-sdk-go v1.44.171 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/docker/docker v23.0.5+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.8.0 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170603005431-491d3605edfb // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc2.0.20221005185240-3a7f492d3f1b // indirect
	github.com/outcaste-io/ristretto v0.2.1 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/client_golang v1.15.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.5.0 // indirect
	github.com/spf13/afero v1.9.3 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.11 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/twmb/murmur3 v1.1.6 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	github.com/zorkian/go-datadog-api v2.30.0+incompatible // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/net v0.9.0 // indirect
	golang.org/x/oauth2 v0.6.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/term v0.7.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	golang.org/x/tools v0.8.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230320184635-7606e756e683 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0 // indirect
	k8s.io/api v0.25.5 // indirect
	k8s.io/apiextensions-apiserver v0.25.5 // indirect
	k8s.io/apiserver v0.25.5 // indirect
	k8s.io/autoscaler/vertical-pod-autoscaler v0.12.0 // indirect
	k8s.io/client-go v0.25.5 // indirect
	k8s.io/component-base v0.25.5 // indirect
	k8s.io/klog/v2 v2.80.1 // indirect
	k8s.io/kube-aggregator v0.23.5 // indirect
	k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280 // indirect
	k8s.io/kubelet v0.25.5 // indirect
	k8s.io/metrics v0.25.5 // indirect
	k8s.io/utils v0.0.0-20221108210102-8e77b1f39fe2 // indirect
	sigs.k8s.io/controller-runtime v0.11.2 // indirect
	sigs.k8s.io/custom-metrics-apiserver v1.25.1 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/DataDog/datadog-agent => ../../
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../obfuscate
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ../util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/log => ../util/log
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../util/scrubber
)
