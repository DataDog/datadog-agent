module github.com/DataDog/datadog-agent

go 1.20

// v0.8.0 was tagged long ago, and appared on pkg.go.dev.  We do not want any tagged version
// to appear there.  The trick to accomplish this is to make a new version (in this case v0.9.0)
// that retracts itself and the previous version.

retract (
	v0.9.0
	v0.8.0
)

// NOTE: Prefer using simple `require` directives instead of using `replace` if possible.
// See https://github.com/DataDog/datadog-agent/blob/main/docs/dev/gomodreplace.md
// for more details.

// Internal deps fix version
replace (
	github.com/cihub/seelog => github.com/cihub/seelog v0.0.0-20151216151435-d2c6e5aa9fbf // v2.6
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20180202092358-40e2722dffea
	github.com/spf13/cast => github.com/DataDog/cast v1.3.1-0.20190301154711-1ee8c8bd14a3
	github.com/ugorji/go => github.com/ugorji/go v1.1.7
)

replace (
	github.com/DataDog/datadog-agent/pkg/gohai => ./pkg/gohai
	github.com/DataDog/datadog-agent/pkg/networkdevice/profile => ./pkg/networkdevice/profile
	github.com/DataDog/datadog-agent/pkg/obfuscate => ./pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/proto => ./pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ./pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/security/secl => ./pkg/security/secl
	github.com/DataDog/datadog-agent/pkg/trace => ./pkg/trace
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ./pkg/util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/log => ./pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/pointer => ./pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ./pkg/util/scrubber
)

require (
	code.cloudfoundry.org/bbs v0.0.0-20200403215808-d7bc971db0db
	code.cloudfoundry.org/garden v0.0.0-20210208153517-580cadd489d2
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/CycloneDX/cyclonedx-go v0.7.1
	github.com/DataDog/appsec-internal-go v1.0.1-0.20230723140106-7cb513f54ce1
	github.com/DataDog/datadog-agent/pkg/gohai v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/security/secl v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/trace v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/log v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.48.0-rc.2
	github.com/DataDog/datadog-go/v5 v5.3.0
	github.com/DataDog/datadog-operator v1.1.0
	github.com/DataDog/ebpf-manager v0.2.16
	github.com/DataDog/go-libddwaf v1.5.0
	github.com/DataDog/go-tuf v1.0.2-0.5.2
	github.com/DataDog/gopsutil v1.2.2
	github.com/DataDog/nikos v1.12.1
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.8.0
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics v0.8.0
	github.com/DataDog/opentelemetry-mapping-go/pkg/quantile v0.8.0
	github.com/DataDog/sketches-go v1.4.2
	github.com/DataDog/viper v1.12.0
	github.com/DataDog/watermarkpodautoscaler v0.5.2
	github.com/DataDog/zstd v1.5.5
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f
	github.com/Masterminds/semver v1.5.0
	github.com/Masterminds/semver/v3 v3.2.1
	github.com/Masterminds/sprig/v3 v3.2.3 // indirect
	github.com/Microsoft/go-winio v0.6.1
	github.com/Microsoft/hcsshim v0.10.0-rc.8
	github.com/acobaugh/osrelease v0.1.0
	github.com/alecthomas/participle v0.7.1 // indirect
	github.com/alecthomas/units v0.0.0-20211218093645-b94a6e3cc137
	github.com/aquasecurity/trivy-db v0.0.0-20230105123735-5ce110fc82e1
	github.com/avast/retry-go/v4 v4.5.0
	github.com/aws/aws-lambda-go v1.37.0
	github.com/aws/aws-sdk-go v1.45.12
	github.com/beevik/ntp v0.3.0
	github.com/benbjohnson/clock v1.3.0
	github.com/bhmj/jsonslice v0.0.0-20200323023432-92c3edaad8e2
	github.com/blabber/go-freebsd-sysctl v0.0.0-20201130114544-503969f39d8f
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cenkalti/backoff/v4 v4.2.1
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/cilium/ebpf v0.11.0
	github.com/clbanning/mxj v1.8.4
	github.com/containerd/containerd v1.7.4
	github.com/containernetworking/cni v1.1.2
	github.com/coreos/go-semver v0.3.0
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf
	github.com/cri-o/ocicni v0.4.0
	github.com/cyphar/filepath-securejoin v0.2.4
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/docker/docker v24.0.2+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/dustin/go-humanize v1.0.1
	github.com/elastic/go-libaudit v0.4.0
	github.com/evanphx/json-patch v5.6.0+incompatible
	github.com/fatih/color v1.15.0
	github.com/freddierice/go-losetup v0.0.0-20170407175016-fc9adea44124
	github.com/fsnotify/fsnotify v1.6.0
	github.com/go-delve/delve v1.20.1
	github.com/go-ini/ini v1.67.0
	github.com/go-ole/go-ole v1.2.6
	github.com/go-redis/redis/v9 v9.0.0-rc.2
	github.com/go-sql-driver/mysql v1.7.1
	github.com/gobwas/glob v0.2.3
	github.com/godbus/dbus v4.1.0+incompatible
	github.com/gogo/protobuf v1.3.2
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.3
	github.com/google/go-cmp v0.5.9
	github.com/google/go-containerregistry v0.15.2
	github.com/google/gofuzz v1.2.0
	github.com/google/gopacket v1.1.19
	github.com/google/pprof v0.0.0-20230602150820-91b7bce49751
	github.com/gorilla/mux v1.8.0
	github.com/gosnmp/gosnmp v1.34.1-0.20220306115220-ca8397b73095
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/grpc-ecosystem/grpc-gateway v1.16.0
	github.com/h2non/filetype v1.1.3
	github.com/hashicorp/consul/api v1.24.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru/v2 v2.0.6
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95
	github.com/iceber/iouring-go v0.0.0-20230403020409-002cfd2e2a90
	github.com/imdario/mergo v0.3.16
	github.com/invopop/jsonschema v0.9.0
	github.com/itchyny/gojq v0.12.13
	github.com/json-iterator/go v1.1.12
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/lxn/walk v0.0.0-20210112085537-c389da54e794
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e
	github.com/mailru/easyjson v0.7.7
	github.com/mdlayher/netlink v1.6.2
	github.com/mholt/archiver/v3 v3.5.1
	github.com/miekg/dns v1.1.51
	github.com/mitchellh/mapstructure v1.5.1-0.20220423185008-bf980b35cac4
	github.com/moby/sys/mountinfo v0.6.2
	github.com/mohae/deepcopy v0.0.0-20170603005431-491d3605edfb
	github.com/netsampler/goflow2 v1.3.3
	github.com/olekukonko/tablewriter v0.0.5
	github.com/oliveagle/jsonpath v0.0.0-20180606110733-2e52cf6e6852
	github.com/open-policy-agent/opa v0.56.0
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.75.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.0-rc4
	github.com/opencontainers/runtime-spec v1.1.0-rc.3
	github.com/openshift/api v3.9.0+incompatible
	github.com/pahanini/go-grpc-bidirectional-streaming-example v0.0.0-20211027164128-cc6111af44be
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.16.0
	github.com/prometheus/client_model v0.4.0
	github.com/prometheus/procfs v0.12.0
	github.com/richardartoul/molecule v1.0.1-0.20221107223329-32cfee06a052
	github.com/robfig/cron/v3 v3.0.1
	github.com/samber/lo v1.38.1
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da
	github.com/secure-systems-lab/go-securesystemslib v0.7.0
	github.com/shirou/gopsutil/v3 v3.23.8
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4
	github.com/sirupsen/logrus v1.9.3
	github.com/skydive-project/go-debouncer v1.0.0
	github.com/smira/go-xz v0.1.0
	github.com/spf13/afero v1.9.5
	github.com/spf13/cast v1.5.1
	github.com/spf13/cobra v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/streadway/amqp v1.1.0
	github.com/stretchr/testify v1.8.4
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	github.com/tinylib/msgp v1.1.8
	github.com/twmb/murmur3 v1.1.8
	github.com/uptrace/bun v1.1.14
	github.com/uptrace/bun/dialect/pgdialect v1.1.14
	github.com/uptrace/bun/driver/pgdriver v1.1.14
	github.com/urfave/negroni v1.0.0
	github.com/vishvananda/netlink v1.2.1-beta.2
	github.com/vishvananda/netns v0.0.0-20220913150850-18c4f4234207
	github.com/vmihailenco/msgpack/v4 v4.3.12
	github.com/wI2L/jsondiff v0.3.0
	github.com/xeipuuv/gojsonschema v1.2.0
	go.etcd.io/bbolt v1.3.7
	go.etcd.io/etcd/client/v2 v2.306.0-alpha.0
	go.mongodb.org/mongo-driver v1.12.1
	go.opentelemetry.io/collector v0.84.0
	go.opentelemetry.io/collector/component v0.84.0
	go.opentelemetry.io/collector/confmap v0.84.0
	go.opentelemetry.io/collector/exporter v0.84.0
	go.opentelemetry.io/collector/exporter/loggingexporter v0.84.0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.84.0
	go.opentelemetry.io/collector/pdata v1.0.0-rcv0014
	go.opentelemetry.io/collector/processor/batchprocessor v0.84.0
	go.opentelemetry.io/collector/receiver v0.84.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.84.0
	go.uber.org/atomic v1.11.0
	go.uber.org/automaxprocs v1.5.3
	go.uber.org/dig v1.17.0
	go.uber.org/fx v1.18.2
	go.uber.org/multierr v1.11.0
	go.uber.org/zap v1.25.0
	go4.org/netipx v0.0.0-20220812043211-3cc044ffd68d
	golang.org/x/arch v0.5.0
	golang.org/x/exp v0.0.0-20230711023510-fffb14384f22
	golang.org/x/net v0.15.0
	golang.org/x/sync v0.3.0
	golang.org/x/sys v0.12.0
	golang.org/x/text v0.13.0
	golang.org/x/time v0.3.0
	golang.org/x/tools v0.13.0
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2
	google.golang.org/genproto v0.0.0-20230706204954-ccb25ca9f130 // indirect
	google.golang.org/grpc v1.57.0
	google.golang.org/grpc/examples v0.0.0-20221020162917-9127159caf5a
	google.golang.org/protobuf v1.31.0
	gopkg.in/DataDog/dd-trace-go.v1 v1.49.1
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0
	k8s.io/api v0.27.6
	k8s.io/apiextensions-apiserver v0.27.6
	k8s.io/apimachinery v0.27.6
	k8s.io/apiserver v0.27.6
	k8s.io/autoscaler/vertical-pod-autoscaler v0.13.0
	k8s.io/client-go v0.27.6
	k8s.io/cri-api v0.27.2
	k8s.io/klog v1.0.1-0.20200310124935-4ad0115ba9e4 // Min version that includes fix for Windows Nano
	k8s.io/klog/v2 v2.100.1
	k8s.io/kube-aggregator v0.27.6
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f
	k8s.io/kube-state-metrics/v2 v2.8.2
	k8s.io/kubelet v0.27.6
	k8s.io/metrics v0.27.6
	k8s.io/utils v0.0.0-20230505201702-9f6742963106
	sigs.k8s.io/custom-metrics-apiserver v1.27.0
)

require (
	cloud.google.com/go v0.110.4 // indirect
	cloud.google.com/go/compute v1.20.1 // indirect
	cloud.google.com/go/compute/metadata v0.2.4-0.20230617002413-005d2dfb6b68 // indirect
	cloud.google.com/go/iam v1.1.0 // indirect
	cloud.google.com/go/storage v1.31.0 // indirect
	code.cloudfoundry.org/cfhttp/v2 v2.0.0 // indirect
	code.cloudfoundry.org/clock v1.0.0 // indirect
	code.cloudfoundry.org/consuladapter v0.0.0-20200131002136-ac1daf48ba97 // indirect
	code.cloudfoundry.org/diego-logging-client v0.0.0-20200130234554-60ef08820a45 // indirect
	code.cloudfoundry.org/executor v0.0.0-20200218194701-024d0bdd52d4 // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20190809170250-f77fb823c7ee // indirect
	code.cloudfoundry.org/go-loggregator v7.4.0+incompatible // indirect
	code.cloudfoundry.org/locket v0.0.0-20200131001124-67fd0a0fdf2d // indirect
	code.cloudfoundry.org/rep v0.0.0-20200325195957-1404b978e31e // indirect
	code.cloudfoundry.org/rfc5424 v0.0.0-20180905210152-236a6d29298a // indirect
	code.cloudfoundry.org/tlsconfig v0.0.0-20200131000646-bbe0f8da39b3 // indirect
	contrib.go.opencensus.io/exporter/prometheus v0.4.2 // indirect
	github.com/AlekSi/pointer v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.28 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.21 // indirect
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.11 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.5 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/DataDog/aptly v1.5.2 // indirect
	github.com/DataDog/extendeddaemonset v0.9.0-rc.2 // indirect
	github.com/DataDog/gostackparse v0.5.0 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DisposaBoy/JsonConfigReader v0.0.0-20201129172854-99cf318d67e7 // indirect
	github.com/GoogleCloudPlatform/docker-credential-gcr v2.0.5+incompatible // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/OneOfOne/xxhash v1.2.8 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20230717121422-5aa5874ade95 // indirect
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/acomagu/bufpipe v1.0.4 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/agnivade/levenshtein v1.1.1 // indirect
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/aquasecurity/go-dep-parser v0.0.0-20230115135733-3be7cb085121 // indirect
	github.com/aquasecurity/go-gem-version v0.0.0-20201115065557-8eed6fe000ce // indirect
	github.com/aquasecurity/go-npm-version v0.0.0-20201110091526-0b796d180798 // indirect
	github.com/aquasecurity/go-pep440-version v0.0.0-20210121094942-22b2f8951d46 // indirect
	github.com/aquasecurity/go-version v0.0.0-20210121072130-637058cfe492 // indirect
	github.com/aquasecurity/table v1.8.0 // indirect
	github.com/aquasecurity/tml v0.6.1 // indirect
	github.com/arduino/go-apt-client v0.0.0-20190812130613-5613f843fdc8 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/awalterschulze/gographviz v2.0.3+incompatible // indirect
	github.com/aws/aws-sdk-go-v2 v1.21.0
	github.com/aws/aws-sdk-go-v2/config v1.18.27 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.13.37
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.13.11 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.41 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.35 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.35 // indirect
	github.com/aws/aws-sdk-go-v2/service/ebs v1.15.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.118.0
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.35 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.13.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.15.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.21.5 // indirect
	github.com/aws/smithy-go v1.14.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmizerany/pat v0.0.0-20170815010413-6226ea591a40 // indirect
	github.com/briandowns/spinner v1.12.0 // indirect
	github.com/caarlos0/env/v6 v6.10.1 // indirect
	github.com/cavaliergopher/grab/v3 v3.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/containerd/cgroups v1.1.0 // indirect
	github.com/containerd/continuity v0.4.2-0.20230616210509-1e0d26eb2381 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.14.3 // indirect
	github.com/containerd/ttrpc v1.2.2 // indirect
	github.com/containerd/typeurl v1.0.2 // indirect
	github.com/containernetworking/plugins v1.2.0 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/dgryski/go-jump v0.0.0-20211018200510-ba001c3ffce0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/docker/cli v23.0.5+incompatible // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-git/gcfg v1.5.0 // indirect
	github.com/go-git/go-billy/v5 v5.3.1 // indirect
	github.com/go-git/go-git/v5 v5.4.2 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.21.4 // indirect
	github.com/go-openapi/errors v0.20.4 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/loads v0.21.2 // indirect
	github.com/go-openapi/runtime v0.26.0 // indirect
	github.com/go-openapi/spec v0.20.9 // indirect
	github.com/go-openapi/strfmt v0.21.7 // indirect
	github.com/go-openapi/swag v0.22.4 // indirect
	github.com/go-openapi/validate v0.22.1 // indirect
	github.com/go-redis/redis/v8 v8.11.5 // indirect
	github.com/go-test/deep v1.0.7 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang/glog v1.1.1 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/licenseclassifier/v2 v2.0.0 // indirect
	github.com/google/uuid v1.3.1
	github.com/google/wire v0.5.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.5 // indirect
	github.com/googleapis/gax-go/v2 v2.12.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.15.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.5.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.4 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/huandu/xstrings v1.3.3 // indirect
	github.com/ianlancetaylor/cgosymbolizer v0.0.0-20221208003206-eaf69f594683
	github.com/ianlancetaylor/demangle v0.0.0-20230524184225-eabc099b10ab // indirect
	github.com/in-toto/in-toto-golang v0.9.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/timefmt-go v0.1.5 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jlaffaye/ftp v0.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jonboulle/clockwork v0.3.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/josharian/native v1.0.0 // indirect
	github.com/justincormack/go-memfd v0.0.0-20170219213707-6e4af0518993
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/kjk/lzma v0.0.0-20161016003348-3fd93898850d // indirect
	github.com/klauspost/compress v1.16.7 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/knadh/koanf v1.5.0 // indirect
	github.com/knqyf263/go-apk-version v0.0.0-20200609155635-041fdbb8563f // indirect
	github.com/knqyf263/go-deb-version v0.0.0-20190517075300-09fca494f03d // indirect
	github.com/knqyf263/go-rpm-version v0.0.0-20220614171824-631e686d1075 // indirect
	github.com/knqyf263/go-rpmdb v0.0.0-20221030142135-919c8a52f04f
	github.com/knqyf263/nested v0.0.1 // indirect
	github.com/liamg/jfather v0.0.7 // indirect
	github.com/libp2p/go-reuseport v0.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/lunixbochs/struc v0.0.0-20200707160740-784aaebc1d40 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/masahiro331/go-disk v0.0.0-20220919035250-c8da316f91ac // indirect
	github.com/masahiro331/go-ebs-file v0.0.0-20221225061409-5ef263bb2cc3 // indirect
	github.com/masahiro331/go-ext4-filesystem v0.0.0-20221225060520-c150f5eacfe1 // indirect
	github.com/masahiro331/go-mvn-version v0.0.0-20210429150710-d3157d602a08 // indirect
	github.com/masahiro331/go-vmdk-parser v0.0.0-20221225061455-612096e4bbbd // indirect
	github.com/masahiro331/go-xfs-filesystem v0.0.0-20221225060805-c02764233454 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mdlayher/socket v0.2.3 // indirect
	github.com/microsoft/go-rustaudit v0.0.0-20220808201409-204dfee52032 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/mkrautz/goar v0.0.0-20150919110319-282caa8bd9da // indirect
	github.com/moby/buildkit v0.11.0 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/sys/signal v0.7.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/montanaflynn/stats v0.0.0-20171201202039-1bf9dbcd8cbe // indirect
	github.com/mostynb/go-grpc-compression v1.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/runc v1.1.5 // indirect
	github.com/opencontainers/selinux v1.11.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/outcaste-io/ristretto v0.2.1 // indirect
	github.com/owenrumney/go-sarif/v2 v2.1.2 // indirect
	github.com/package-url/packageurl-go v0.1.1-0.20220428063043-89078438f170 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pierrec/lz4/v4 v4.1.18 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/common v0.44.0
	github.com/prometheus/statsd_exporter v0.22.7 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20200410134404-eec4a21b6bb0 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/rs/cors v1.9.0 // indirect
	github.com/safchain/baloum v0.0.0-20221229104256-b1fc8f70a86b
	github.com/saracen/walker v0.0.0-20191201085201-324a081bae7e // indirect
	github.com/sassoftware/go-rpmutils v0.2.0 // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/shibumi/go-pathspec v1.3.0 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/smira/go-ftp-protocol v0.0.0-20140829150050-066b75c2b70d // indirect
	github.com/spdx/tools-golang v0.3.1-0.20230104082527-d6f58551be3f // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d // indirect
	github.com/tchap/go-patricia/v2 v2.3.1 // indirect
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00 // indirect
	github.com/tedsuo/rata v1.0.0 // indirect
	github.com/tidwall/gjson v1.14.3 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/tmthrgd/go-hex v0.0.0-20190904060850-447a3041c3bc // indirect
	github.com/twitchtv/twirp v8.1.2+incompatible // indirect
	github.com/twmb/franz-go v1.14.3
	github.com/twmb/franz-go/pkg/kadm v1.9.0
	github.com/twmb/franz-go/pkg/kmsg v1.6.1 // indirect
	github.com/ugorji/go/codec v1.2.7 // indirect
	github.com/ulikunitz/xz v0.5.11 // indirect
	github.com/vbatts/tar-split v0.11.3 // indirect
	github.com/vito/go-sse v1.0.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/xlab/treeprint v1.1.0 // indirect
	github.com/xor-gate/ar v0.0.0-20170530204233-5c72ae81e2b7 // indirect
	github.com/yashtewari/glob-intersection v0.2.0 // indirect
	github.com/youmark/pkcs8 v0.0.0-20181117223130-1be2e3e5546d // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.etcd.io/etcd/api/v3 v3.6.0-alpha.0 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.0-alpha.0.0.20220522111935-c3bc4116dcd1 // indirect
	go.etcd.io/etcd/client/v3 v3.6.0-alpha.0 // indirect
	go.etcd.io/etcd/server/v3 v3.6.0-alpha.0.0.20220522111935-c3bc4116dcd1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector/consumer v0.84.0
	go.opentelemetry.io/collector/featuregate v1.0.0-rcv0014 // indirect
	go.opentelemetry.io/collector/semconv v0.84.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.42.1-0.20230612162650-64be7e574a17 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.42.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.17.0 // indirect
	go.opentelemetry.io/otel v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.16.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.16.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.16.0
	go.opentelemetry.io/otel/exporters/prometheus v0.39.0
	go.opentelemetry.io/otel/metric v1.16.0
	go.opentelemetry.io/otel/sdk v1.16.0
	go.opentelemetry.io/otel/sdk/metric v0.39.0
	go.opentelemetry.io/otel/trace v1.16.0 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	golang.org/x/crypto v0.13.0 // indirect
	golang.org/x/mod v0.12.0
	golang.org/x/oauth2 v0.10.0 // indirect
	golang.org/x/term v0.12.0 // indirect
	gonum.org/v1/gonum v0.14.0 // indirect
	google.golang.org/api v0.134.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/Knetic/govaluate.v3 v3.0.0 // indirect
	gopkg.in/cheggaaa/pb.v1 v1.0.28 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	k8s.io/component-base v0.27.6
	k8s.io/gengo v0.0.0-20220902162205-c0856e24416d // indirect
	lukechampine.com/uint128 v1.2.0 // indirect
	mellium.im/sasl v0.3.1 // indirect
	modernc.org/cc/v3 v3.40.0 // indirect
	modernc.org/ccgo/v3 v3.16.13-0.20221017192402-261537637ce8 // indirect
	modernc.org/libc v1.21.2 // indirect
	modernc.org/mathutil v1.5.0 // indirect
	modernc.org/memory v1.4.0 // indirect
	modernc.org/opt v0.1.3 // indirect
	modernc.org/sqlite v1.17.3 // indirect
	modernc.org/strutil v1.1.3 // indirect
	modernc.org/token v1.0.1 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.1.2 // indirect
	sigs.k8s.io/controller-runtime v0.11.2 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0
)

require github.com/lorenzosaino/go-sysctl v0.3.1

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/DataDog/agent-payload/v5 v5.0.96
	github.com/DataDog/datadog-agent/pkg/networkdevice/profile v0.0.0-20230913122404-c12f437d52a4
	github.com/DataDog/datadog-agent/pkg/proto v0.48.0-rc.2
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/logs v0.8.0
	github.com/aquasecurity/trivy v0.0.0-00010101000000-000000000000
	github.com/cloudfoundry-community/go-cfclient/v2 v2.0.1-0.20230503155151-3d15366c5820
	github.com/containerd/cgroups/v3 v3.0.2
	github.com/containerd/typeurl/v2 v2.1.1
	github.com/gocomply/scap v0.1.2-0.20230531064509-55a00f73e8d6
	github.com/godror/godror v0.37.0
	github.com/jmoiron/sqlx v1.3.5
	github.com/kr/pretty v0.3.1
	github.com/protocolbuffers/protoscope v0.0.0-20221109213918-8e7a6aafa2c9
	github.com/sijms/go-ora/v2 v2.7.6
	go.opentelemetry.io/collector/extension v0.84.0
	go.opentelemetry.io/collector/processor v0.84.0
	go4.org/intern v0.0.0-20211027215823-ae77deb06f29
)

require (
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20230811130428-ced1acdcaa24 // indirect
	github.com/AdamKorcz/go-118-fuzz-build v0.0.0-20230306123547-8075edf89bb0 // indirect
	github.com/DataDog/datadog-api-client-go/v2 v2.13.0 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v1.4.10 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cloudflare/circl v1.3.3 // indirect
	github.com/ebitengine/purego v0.5.0-alpha.0.20230803142445-813050efbc99 // indirect
	github.com/emicklei/go-restful/v3 v3.10.2 // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/godror/knownpb v0.1.0 // indirect
	github.com/google/cel-go v0.12.7 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/s2a-go v0.1.4 // indirect
	github.com/hashicorp/hcl/v2 v2.17.0 // indirect
	github.com/knadh/koanf/v2 v2.0.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/moby/sys/sequential v0.5.0 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	github.com/rs/zerolog v1.29.1 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/sigstore/rekor v1.2.2 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/zclconf/go-cty v1.13.0 // indirect
	go.opentelemetry.io/collector/config/configauth v0.84.0 // indirect
	go.opentelemetry.io/collector/config/configcompression v0.84.0 // indirect
	go.opentelemetry.io/collector/config/configgrpc v0.84.0 // indirect
	go.opentelemetry.io/collector/config/confighttp v0.84.0 // indirect
	go.opentelemetry.io/collector/config/confignet v0.84.0 // indirect
	go.opentelemetry.io/collector/config/configopaque v0.84.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.84.0 // indirect
	go.opentelemetry.io/collector/config/configtls v0.84.0 // indirect
	go.opentelemetry.io/collector/config/internal v0.84.0 // indirect
	go.opentelemetry.io/collector/connector v0.84.0 // indirect
	go.opentelemetry.io/collector/extension/auth v0.84.0 // indirect
	go.opentelemetry.io/otel/bridge/opencensus v0.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric v0.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v0.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v0.39.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v0.39.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.16.0 // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20230525183740-e7c30c78aeb2 // indirect
	golang.org/x/exp/typeparams v0.0.0-20220613132600-b0d781184e0d // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	gomodules.xyz/jsonpatch/v2 v2.3.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230706204954-ccb25ca9f130 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230720185612-659f7aaaa771 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	honnef.co/go/tools v0.3.2 // indirect
	inet.af/netaddr v0.0.0-20220811202034-502d2d690317 // indirect
	k8s.io/kms v0.27.6 // indirect
)

replace github.com/pahanini/go-grpc-bidirectional-streaming-example v0.0.0-20211027164128-cc6111af44be => github.com/DataDog/go-grpc-bidirectional-streaming-example v0.0.0-20221024060302-b9cf785c02fe

// Fixing a CVE on a transitive dep of k8s/etcd, should be cleaned-up once k8s.io/apiserver dep is removed (but double-check with `go mod why` that no other dep pulls it)
replace github.com/dgrijalva/jwt-go => github.com/golang-jwt/jwt v3.2.1+incompatible

replace github.com/vishvananda/netlink => github.com/DataDog/netlink v1.0.1-0.20230703150631-f11d5ab05838

// Cannot be upgraded to 0.26 without lossing CRI API v1alpha2
replace k8s.io/cri-api => k8s.io/cri-api v0.25.5

// Use custom Trivy fork to reduce binary size
// Pull in replacements needed by upstream Trivy
replace (
	github.com/aquasecurity/trivy => github.com/DataDog/trivy v0.0.0-20230526171704-5aaa4575395e
	github.com/saracen/walker => github.com/DataDog/walker v0.0.0-20230418153152-7f29bb2dc950
	github.com/spdx/tools-golang => github.com/spdx/tools-golang v0.3.0
	oras.land/oras-go => oras.land/oras-go v1.1.1
)

// Waiting for datadog-operator kube version bump
replace sigs.k8s.io/controller-runtime v0.11.2 => sigs.k8s.io/controller-runtime v0.15.0

// Fixes CVE-2023-1732, imported by nikos
replace github.com/cloudflare/circl v1.1.0 => github.com/cloudflare/circl v1.3.3

// Fixes CVE-2023-26054, imported by trivy
replace github.com/moby/buildkit v0.11.0 => github.com/moby/buildkit v0.11.4

// Fixes a panic in trivy, see gitlab.com/cznic/libc/-/issues/25
replace modernc.org/sqlite v1.17.3 => modernc.org/sqlite v1.19.3

// Exclude specific versions of knadh/koanf to fix building with a `go.work`, following
// https://github.com/open-telemetry/opentelemetry-collector/issues/8127
exclude (
	github.com/knadh/koanf/maps v0.1.1
	github.com/knadh/koanf/providers/confmap v0.1.0
)
