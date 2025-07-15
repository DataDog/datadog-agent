module github.com/DataDog/datadog-agent

go 1.24.0

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
	// Use a patched version of go-cmp to avoid disabling dead code elimination
	// Commit from https://github.com/DataDog/go-cmp/tree/dce-patch/v0.7.0
	github.com/google/go-cmp => github.com/DataDog/go-cmp v0.0.0-20250605161605-8f326bf2ab9d
	github.com/spf13/cast => github.com/DataDog/cast v1.8.0
)

require (
	code.cloudfoundry.org/bbs v0.0.0-20200403215808-d7bc971db0db
	code.cloudfoundry.org/garden v0.0.0-20210208153517-580cadd489d2
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/CycloneDX/cyclonedx-go v0.9.2
	github.com/DataDog/appsec-internal-go v1.12.0
	github.com/DataDog/datadog-agent/pkg/gohai v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.64.0-rc.12
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.64.0-rc.12
	github.com/DataDog/datadog-agent/pkg/security/secl v0.56.0
	github.com/DataDog/datadog-agent/pkg/trace v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.64.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/log v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.67.0
	github.com/DataDog/datadog-go/v5 v5.6.0
	// TODO: pin to an operator released version once there is a release that includes the api module
	github.com/DataDog/datadog-operator/api v0.0.0-20250417130148-1aa8dc0fc964
	github.com/DataDog/ebpf-manager v0.7.12
	github.com/DataDog/gopsutil v1.2.2
	github.com/DataDog/nikos v1.12.12
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.29.0
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics v0.29.0
	github.com/DataDog/opentelemetry-mapping-go/pkg/quantile v0.29.0
	github.com/DataDog/sketches-go v1.4.7
	github.com/DataDog/viper v1.14.1-0.20250612143030-1b15c8822ed4
	// TODO: pin to a WPA released version once there is a release that includes the apis module
	github.com/DataDog/watermarkpodautoscaler/apis v0.0.0-20250108152814-82e58d0231d1
	github.com/DataDog/zstd v1.5.6
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/Masterminds/semver/v3 v3.3.1
	github.com/Microsoft/go-winio v0.6.2
	github.com/Microsoft/hcsshim v0.12.9
	github.com/acobaugh/osrelease v0.1.0
	github.com/alecthomas/participle v0.7.1 // indirect
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b
	github.com/aquasecurity/trivy-db v0.0.0-20250227071930-8bd8a9b89e2d
	github.com/avast/retry-go/v4 v4.6.0
	github.com/aws/aws-lambda-go v1.37.0
	github.com/aws/aws-sdk-go v1.55.7 // indirect
	github.com/beevik/ntp v1.4.3
	github.com/benbjohnson/clock v1.3.5
	github.com/bhmj/jsonslice v1.1.3
	github.com/blabber/go-freebsd-sysctl v0.0.0-20201130114544-503969f39d8f
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/cilium/ebpf v0.19.0
	github.com/clbanning/mxj v1.8.4
	github.com/containerd/containerd v1.7.27
	github.com/containernetworking/cni v1.2.3
	github.com/coreos/go-semver v0.3.1
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/cri-o/ocicni v0.4.3
	github.com/cyphar/filepath-securejoin v0.4.1
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/docker/docker v28.1.1+incompatible
	github.com/docker/go-connections v0.5.0
	github.com/dustin/go-humanize v1.0.1
	github.com/elastic/go-libaudit/v2 v2.5.0
	github.com/evanphx/json-patch v5.9.11+incompatible
	github.com/fatih/color v1.18.0
	github.com/freddierice/go-losetup v0.0.0-20220711213114-2a14873012db
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-delve/delve v1.24.0
	github.com/go-ini/ini v1.67.0
	github.com/go-ole/go-ole v1.3.0
	github.com/go-sql-driver/mysql v1.8.1
	github.com/gobwas/glob v0.2.3
	github.com/gogo/protobuf v1.3.2
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8
	github.com/golang/mock v1.7.0-rc.1
	github.com/golang/protobuf v1.5.4
	github.com/google/go-cmp v0.7.0
	github.com/google/go-containerregistry v0.20.3
	github.com/google/gofuzz v1.2.0
	github.com/google/gopacket v1.1.19
	github.com/google/pprof v0.0.0-20250317173921-a4b03ec1a45e // indirect
	github.com/gorilla/mux v1.8.1
	github.com/gosnmp/gosnmp v1.38.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/h2non/filetype v1.1.3
	github.com/hashicorp/consul/api v1.32.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb
	github.com/iceber/iouring-go v0.0.0-20230403020409-002cfd2e2a90
	github.com/imdario/mergo v0.3.16
	github.com/invopop/jsonschema v0.12.0
	github.com/itchyny/gojq v0.12.16
	github.com/json-iterator/go v1.1.12
	github.com/lxn/walk v0.0.0-20210112085537-c389da54e794
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e
	github.com/mailru/easyjson v0.9.0
	github.com/mdlayher/netlink v1.7.2
	github.com/miekg/dns v1.1.65
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c
	github.com/moby/sys/mountinfo v0.7.2
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/netsampler/goflow2 v1.3.3
	github.com/olekukonko/tablewriter v0.0.5
	github.com/oliveagle/jsonpath v0.0.0-20180606110733-2e52cf6e6852
	github.com/open-policy-agent/opa v1.4.2
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.129.0 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/opencontainers/runtime-spec v1.2.1
	github.com/openshift/api v3.9.0+incompatible
	github.com/pahanini/go-grpc-bidirectional-streaming-example v0.0.0-20211027164128-cc6111af44be
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.22.0
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/procfs v0.16.1
	github.com/redis/go-redis/v9 v9.8.0
	github.com/richardartoul/molecule v1.0.1-0.20240531184615-7ca0df43c0b3 // indirect
	github.com/robfig/cron/v3 v3.0.1
	github.com/samber/lo v1.49.1
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4
	github.com/sirupsen/logrus v1.9.3
	github.com/skydive-project/go-debouncer v1.0.1
	github.com/smira/go-xz v0.1.0
	github.com/spf13/afero v1.14.0
	github.com/spf13/cast v1.9.2
	github.com/spf13/cobra v1.9.1
	github.com/spf13/pflag v1.0.6
	github.com/streadway/amqp v1.1.0
	github.com/stretchr/testify v1.10.0
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	github.com/tinylib/msgp v1.3.0
	github.com/twmb/murmur3 v1.1.8
	github.com/uptrace/bun v1.2.5
	github.com/uptrace/bun/dialect/pgdialect v1.2.5
	github.com/uptrace/bun/driver/pgdriver v1.2.5
	github.com/urfave/negroni v1.0.0
	github.com/vishvananda/netlink v1.3.0
	github.com/vishvananda/netns v0.0.5
	github.com/vmihailenco/msgpack/v4 v4.3.13
	github.com/wI2L/jsondiff v0.6.1
	github.com/xeipuuv/gojsonschema v1.2.0
	go.etcd.io/bbolt v1.4.0
	go.etcd.io/etcd/client/v2 v2.306.0-alpha.0
	go.mongodb.org/mongo-driver/v2 v2.1.0
	go.opentelemetry.io/collector v0.129.0 // indirect
	go.opentelemetry.io/collector/component v1.35.0
	go.opentelemetry.io/collector/confmap v1.35.0
	go.opentelemetry.io/collector/exporter v0.129.0
	go.opentelemetry.io/collector/exporter/debugexporter v0.129.0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.129.0
	go.opentelemetry.io/collector/pdata v1.35.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.129.0
	go.opentelemetry.io/collector/receiver v1.35.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.129.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.61.0 // indirect
	go.uber.org/atomic v1.11.0
	go.uber.org/automaxprocs v1.6.0
	go.uber.org/dig v1.19.0
	go.uber.org/fx v1.24.0
	go.uber.org/multierr v1.11.0
	go.uber.org/zap v1.27.0
	go4.org/netipx v0.0.0-20220812043211-3cc044ffd68d
	golang.org/x/arch v0.18.0
	golang.org/x/exp v0.0.0-20250606033433-dcc06ee1d476
	golang.org/x/net v0.41.0
	golang.org/x/sync v0.15.0
	golang.org/x/sys v0.33.0
	golang.org/x/text v0.26.0
	golang.org/x/time v0.12.0
	golang.org/x/tools v0.34.0
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da
	google.golang.org/genproto v0.0.0-20250425173222-7b384671a197 // indirect
	google.golang.org/grpc v1.73.0
	google.golang.org/grpc/examples v0.0.0-20221020162917-9127159caf5a
	google.golang.org/protobuf v1.36.6
	gopkg.in/DataDog/dd-trace-go.v1 v1.72.2
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0
	k8s.io/api v0.32.3
	k8s.io/apiextensions-apiserver v0.32.1
	k8s.io/apimachinery v0.32.3
	k8s.io/apiserver v0.32.1 // indirect
	k8s.io/autoscaler/vertical-pod-autoscaler v1.2.2
	k8s.io/client-go v0.32.3
	k8s.io/cri-api v0.31.2
	k8s.io/klog/v2 v2.130.1
	k8s.io/kube-aggregator v0.31.2
	k8s.io/kube-openapi v0.0.0-20241105132330-32ad38e42d3f // indirect
	k8s.io/kube-state-metrics/v2 v2.13.1-0.20241025121156-110f03d7331f
	k8s.io/kubelet v0.31.2
	k8s.io/metrics v0.31.2
	k8s.io/utils v0.0.0-20241104100929-3ea5e8cea738
	sigs.k8s.io/custom-metrics-apiserver v1.30.1-0.20241105195130-84dc8cfe2555
)

require (
	cloud.google.com/go/compute/metadata v0.7.0 // indirect
	code.cloudfoundry.org/cfhttp/v2 v2.0.0 // indirect
	code.cloudfoundry.org/clock v1.0.0 // indirect
	code.cloudfoundry.org/consuladapter v0.0.0-20200131002136-ac1daf48ba97 // indirect
	code.cloudfoundry.org/diego-logging-client v0.0.0-20200130234554-60ef08820a45 // indirect
	code.cloudfoundry.org/executor v0.0.0-20200218194701-024d0bdd52d4 // indirect
	code.cloudfoundry.org/locket v0.0.0-20200131001124-67fd0a0fdf2d // indirect
	code.cloudfoundry.org/rep v0.0.0-20200325195957-1404b978e31e // indirect
	code.cloudfoundry.org/tlsconfig v0.0.0-20200131000646-bbe0f8da39b3 // indirect
	github.com/AlekSi/pointer v1.2.0 // indirect
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/DataDog/aptly v1.5.3 // indirect
	github.com/DataDog/go-tuf v1.1.0-0.5.2 // indirect
	github.com/DataDog/gostackparse v0.7.0 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DisposaBoy/JsonConfigReader v0.0.0-20201129172854-99cf318d67e7 // indirect
	github.com/Masterminds/semver v1.5.0
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/ProtonMail/go-crypto v1.1.6
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/awalterschulze/gographviz v2.0.3+incompatible // indirect
	github.com/aws/aws-sdk-go-v2 v1.36.5
	github.com/aws/aws-sdk-go-v2/config v1.29.17
	github.com/aws/aws-sdk-go-v2/credentials v1.17.70
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.226.0
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.34.0 // indirect
	github.com/aws/smithy-go v1.22.4 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmizerany/pat v0.0.0-20170815010413-6226ea591a40 // indirect
	github.com/briandowns/spinner v1.23.0 // indirect
	github.com/cavaliergopher/grab/v3 v3.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/continuity v0.4.5 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.16.3 // indirect
	github.com/containerd/ttrpc v1.2.7 // indirect
	github.com/containernetworking/plugins v1.4.1 // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/dgryski/go-jump v0.0.0-20211018200510-ba001c3ffce0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/docker/cli v27.5.0+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.2 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-redis/redis/v8 v8.11.5 // indirect
	github.com/godbus/dbus/v5 v5.1.0
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/uuid v1.6.0
	github.com/google/wire v0.6.0 // indirect
	github.com/googleapis/gax-go/v2 v2.14.1 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-version v1.7.0
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-7 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/timefmt-go v0.1.6 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jlaffaye/ftp v0.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/justincormack/go-memfd v0.0.0-20170219213707-6e4af0518993
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kjk/lzma v0.0.0-20161016003348-3fd93898850d // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/knqyf263/go-deb-version v0.0.0-20241115132648-6f4aee6ccd23 // indirect
	github.com/knqyf263/go-rpm-version v0.0.0-20220614171824-631e686d1075 // indirect
	github.com/knqyf263/go-rpmdb v0.1.2-0.20241125135340-7670f0f23c16
	github.com/libp2p/go-reuseport v0.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20240909124753-873cd0166683 // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/masahiro331/go-disk v0.0.0-20240625071113-56c933208fee // indirect
	github.com/masahiro331/go-ext4-filesystem v0.0.0-20240620024024-ca14e6327bbd // indirect
	github.com/masahiro331/go-xfs-filesystem v0.0.0-20231205045356-1b22259a6c44 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mdlayher/socket v0.5.0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/mkrautz/goar v0.0.0-20150919110319-282caa8bd9da // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/sys/signal v0.7.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/mostynb/go-grpc-compression v1.2.3 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/opencontainers/selinux v1.11.1 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/package-url/packageurl-go v0.1.3 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pierrec/lz4/v4 v4.1.22
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/common v0.65.0
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/safchain/baloum v0.0.0-20241120122234-f22c9bd19f3b
	github.com/sassoftware/go-rpmutils v0.4.0 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.9.0 // indirect
	github.com/smira/go-ftp-protocol v0.0.0-20140829150050-066b75c2b70d // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tchap/go-patricia/v2 v2.3.2 // indirect
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00 // indirect
	github.com/tedsuo/rata v1.0.0 // indirect
	github.com/tklauser/go-sysconf v0.3.14
	github.com/tklauser/numcpus v0.9.0 // indirect
	github.com/tmthrgd/go-hex v0.0.0-20190904060850-447a3041c3bc // indirect
	github.com/twitchtv/twirp v8.1.3+incompatible // indirect
	github.com/twmb/franz-go v1.18.2-0.20250413173443-1d5a55fa468d
	github.com/twmb/franz-go/pkg/kadm v1.12.0
	github.com/twmb/franz-go/pkg/kmsg v1.11.2-0.20250413173443-1d5a55fa468d
	github.com/ugorji/go/codec v1.2.11 // indirect
	github.com/ulikunitz/xz v0.5.12 // indirect
	github.com/vbatts/tar-split v0.11.6 // indirect
	github.com/vito/go-sse v1.0.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8
	github.com/xor-gate/ar v0.0.0-20170530204233-5c72ae81e2b7 // indirect
	github.com/yashtewari/glob-intersection v0.2.0 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	github.com/yusufpapurcu/wmi v1.2.4
	go.etcd.io/etcd/api/v3 v3.6.0-alpha.0 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.0-alpha.0.0.20220522111935-c3bc4116dcd1 // indirect
	go.etcd.io/etcd/client/v3 v3.6.0-alpha.0 // indirect
	go.etcd.io/etcd/server/v3 v3.6.0-alpha.0.0.20220522111935-c3bc4116dcd1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector/consumer v1.35.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.35.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.36.0 // indirect
	go.opentelemetry.io/otel v1.36.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.36.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.36.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.36.0
	go.opentelemetry.io/otel/exporters/prometheus v0.58.0 // indirect
	go.opentelemetry.io/otel/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/sdk v1.36.0
	go.opentelemetry.io/otel/sdk/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0
	go.opentelemetry.io/proto/otlp v1.6.0 // indirect
	golang.org/x/crypto v0.39.0 // indirect
	golang.org/x/mod v0.25.0
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	gonum.org/v1/gonum v0.16.0 // indirect
	google.golang.org/api v0.230.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	gopkg.in/Knetic/govaluate.v3 v3.0.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	k8s.io/component-base v0.32.1
	mellium.im/sasl v0.3.2 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.31.0 // indirect
	sigs.k8s.io/controller-runtime v0.20.4 // indirect
	sigs.k8s.io/json v0.0.0-20241010143419-9aa6b5e7a4b3 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.5.0 // indirect
	sigs.k8s.io/yaml v1.4.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.67.0
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.67.0
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def v0.59.0-rc.6
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types v0.65.0-devel
	github.com/DataDog/datadog-agent/pkg/config/structure v0.67.0
	github.com/DataDog/datadog-agent/pkg/fips v0.67.0
	github.com/DataDog/datadog-agent/pkg/network/payload v0.0.0-20250128160050-7ac9ccd58c07
	github.com/DataDog/datadog-agent/pkg/networkpath/payload v0.0.0-20250128160050-7ac9ccd58c07
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths v0.64.0-devel
	github.com/DataDog/datadog-agent/pkg/util/utilizationtracker v0.0.0
	github.com/DataDog/dd-trace-go/v2 v2.0.0
	github.com/NVIDIA/go-nvml v0.12.4-0
	github.com/cloudflare/cbpfc v0.0.0-20240920015331-ff978e94500b
	github.com/containerd/containerd/api v1.8.0
	github.com/containerd/errdefs v1.0.0
	github.com/distribution/reference v0.6.0
	github.com/expr-lang/expr v1.17.5 // indirect
	github.com/go-viper/mapstructure/v2 v2.3.0
	github.com/jellydator/ttlcache/v3 v3.4.0
	github.com/kouhin/envflag v0.0.0-20150818174321-0e9a86061649
	github.com/lorenzosaino/go-sysctl v0.3.1
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog v0.129.0
	go.opentelemetry.io/collector/config/configtelemetry v0.129.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/configsync v0.64.0
	github.com/DataDog/datadog-agent/comp/core/ipc/def v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/core/ipc/impl v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/core/ipc/mock v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/core/log/fx v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/tagger/def v0.64.1
	github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/tagger/generic_store v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/tagger/subscriber v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry v0.64.1
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def v0.64.0
	github.com/DataDog/datadog-agent/comp/otelcol/status/def v0.64.0
	github.com/DataDog/datadog-agent/comp/otelcol/status/impl v0.64.0
	github.com/DataDog/datadog-agent/pkg/fleet/installer v0.64.0
	github.com/DataDog/datadog-agent/pkg/util/compression v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/otel v0.64.0
	github.com/DataDog/datadog-agent/pkg/util/prometheus v0.64.0
	github.com/Masterminds/sprig/v3 v3.3.0
	github.com/coreos/go-systemd v0.0.0-20190321100706-95778dfbb74e
	github.com/kraken-hpc/go-fork v0.1.1
	github.com/mattn/go-sqlite3 v1.14.24
	github.com/safchain/ethtool v0.5.10
	github.com/shirou/gopsutil/v4 v4.25.5
	go.opentelemetry.io/collector/component/componenttest v0.129.0
	modernc.org/sqlite v1.36.2 // indirect
)

require (
	github.com/bboreham/go-loser v0.0.0-20230920113527-fcc2c21820a3 // indirect
	github.com/edsrzf/mmap-go v1.2.0 // indirect
	github.com/facette/natsort v0.0.0-20181210072756-2cd4dd1e2dcb // indirect
	github.com/go-openapi/analysis v0.23.0 // indirect
	github.com/go-openapi/errors v0.22.0 // indirect
	github.com/go-openapi/loads v0.22.0 // indirect
	github.com/go-openapi/spec v0.21.0 // indirect
	github.com/go-openapi/strfmt v0.23.0 // indirect
	github.com/go-openapi/validate v0.24.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/mdlayher/vsock v1.2.1 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/prometheus/alertmanager v0.28.1 // indirect
	github.com/prometheus/common/assets v0.2.0 // indirect
	github.com/prometheus/exporter-toolkit v0.14.0 // indirect
	github.com/shurcooL/httpfs v0.0.0-20230704072500-f1e31cf0ba5c // indirect
	go.mongodb.org/mongo-driver v1.14.0 // indirect
	go.opentelemetry.io/collector/processor/processorhelper v0.129.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace v0.60.0 // indirect
	go.uber.org/goleak v1.3.0 // indirect
)

require (
	github.com/envoyproxy/go-control-plane/envoy v1.32.4 // indirect
	go.opentelemetry.io/collector/receiver/receiverhelper v0.129.0 // indirect
	go.opentelemetry.io/contrib/otelconf v0.16.0 // indirect
)

require (
	github.com/cenkalti/backoff/v5 v5.0.2 // indirect
	go.opentelemetry.io/collector/extension/extensionauth v1.35.0 // indirect
	go.opentelemetry.io/collector/service/hostcapabilities v0.129.0 // indirect
)

require (
	github.com/jaegertracing/jaeger-idl v0.5.0 // indirect
	go.opentelemetry.io/collector/confmap/xconfmap v0.129.0 // indirect
	go.opentelemetry.io/collector/internal/telemetry v0.129.0 // indirect
	go.uber.org/zap/exp v0.3.0 // indirect
)

require (
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/core/xidutils v0.129.0 // indirect
	github.com/tilinna/clock v1.1.0 // indirect
	go.opentelemetry.io/collector/connector/xconnector v0.129.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror/xconsumererror v0.129.0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.129.0 // indirect
	go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper v0.129.0 // indirect
	go.opentelemetry.io/collector/exporter/xexporter v0.129.0 // indirect
	go.opentelemetry.io/collector/extension/extensiontest v0.129.0 // indirect
	go.opentelemetry.io/collector/extension/xextension v0.129.0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.129.0 // indirect
	go.opentelemetry.io/collector/processor/processorhelper/xprocessorhelper v0.129.0 // indirect
	go.opentelemetry.io/collector/processor/xprocessor v0.129.0 // indirect
	go.opentelemetry.io/collector/receiver/xreceiver v0.129.0 // indirect
	go.opentelemetry.io/collector/scraper v0.129.0 // indirect
	go.opentelemetry.io/collector/scraper/scraperhelper v0.129.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.12.2 // indirect
)

require (
	go.opentelemetry.io/collector/connector/connectortest v0.129.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror v0.129.0 // indirect
	go.opentelemetry.io/collector/exporter/exportertest v0.129.0 // indirect
	go.opentelemetry.io/collector/internal/fanoutconsumer v0.129.0 // indirect
	go.opentelemetry.io/collector/internal/memorylimiter v0.129.0 // indirect
	go.opentelemetry.io/collector/internal/sharedcomponent v0.129.0 // indirect
	go.opentelemetry.io/collector/processor/processortest v0.129.0 // indirect
	go.opentelemetry.io/collector/receiver/receivertest v0.129.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.11.0 // indirect
)

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/DataDog/agent-payload/v5 v5.0.158
	github.com/DataDog/datadog-agent/comp/api/api/def v0.61.0
	github.com/DataDog/datadog-agent/comp/core/config v0.67.0
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.67.0
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface v0.64.0-rc.3
	github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/core/log/def v0.67.0
	github.com/DataDog/datadog-agent/comp/core/log/impl v0.61.0
	github.com/DataDog/datadog-agent/comp/core/log/impl-trace v0.59.0
	github.com/DataDog/datadog-agent/comp/core/log/mock v0.64.0-devel
	github.com/DataDog/datadog-agent/comp/core/secrets v0.67.0
	github.com/DataDog/datadog-agent/comp/core/status v0.67.0
	github.com/DataDog/datadog-agent/comp/core/status/statusimpl v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection v0.67.0
	github.com/DataDog/datadog-agent/comp/core/tagger/tags v0.64.0-devel
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.67.0
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.67.0
	github.com/DataDog/datadog-agent/comp/def v0.67.0
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.67.0
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface v0.67.0
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.67.0
	github.com/DataDog/datadog-agent/comp/netflow/payload v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def v0.64.0
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl v0.58.0
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline v0.64.0-rc.12
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/connector/datadogconnector v0.65.0-devel
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter v0.59.0
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter v0.64.0-devel.0.20250218192636-64fdfe7ec366
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter v0.65.0-devel.0.20250304124125-23a109221842
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient v0.64.0-rc.12
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor v0.59.0
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil v0.67.0
	github.com/DataDog/datadog-agent/comp/serializer/logscompression v0.64.0-rc.12
	github.com/DataDog/datadog-agent/comp/serializer/metricscompression v0.67.0
	github.com/DataDog/datadog-agent/comp/trace/agent/def v0.61.0
	github.com/DataDog/datadog-agent/comp/trace/compression/def v0.64.0-rc.12
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip v0.64.0-rc.12
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey v0.67.0
	github.com/DataDog/datadog-agent/pkg/api v0.67.0
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.67.0
	github.com/DataDog/datadog-agent/pkg/config/create v0.67.0
	github.com/DataDog/datadog-agent/pkg/config/env v0.67.0
	github.com/DataDog/datadog-agent/pkg/config/mock v0.67.0
	github.com/DataDog/datadog-agent/pkg/config/model v0.67.0
	github.com/DataDog/datadog-agent/pkg/config/remote v0.59.0-rc.5
	github.com/DataDog/datadog-agent/pkg/config/setup v0.67.0
	github.com/DataDog/datadog-agent/pkg/config/utils v0.67.0
	github.com/DataDog/datadog-agent/pkg/errors v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/logs/client v0.64.0-rc.12
	github.com/DataDog/datadog-agent/pkg/logs/diagnostic v0.64.0-rc.12
	github.com/DataDog/datadog-agent/pkg/logs/message v0.64.0-rc.12
	github.com/DataDog/datadog-agent/pkg/logs/metrics v0.64.0-rc.12
	github.com/DataDog/datadog-agent/pkg/logs/pipeline v0.64.0-rc.3
	github.com/DataDog/datadog-agent/pkg/logs/processor v0.64.0-rc.3
	github.com/DataDog/datadog-agent/pkg/logs/sender v0.64.0-rc.12
	github.com/DataDog/datadog-agent/pkg/logs/sources v0.64.0-rc.12
	github.com/DataDog/datadog-agent/pkg/logs/status/utils v0.64.0-rc.12
	github.com/DataDog/datadog-agent/pkg/logs/util/testutils v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/metrics v0.67.0
	github.com/DataDog/datadog-agent/pkg/networkdevice/profile v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.67.0
	github.com/DataDog/datadog-agent/pkg/process/util/api v0.67.0
	github.com/DataDog/datadog-agent/pkg/proto v0.68.0-devel
	github.com/DataDog/datadog-agent/pkg/security/seclwin v0.56.0
	github.com/DataDog/datadog-agent/pkg/serializer v0.67.0
	github.com/DataDog/datadog-agent/pkg/status/health v0.67.0
	github.com/DataDog/datadog-agent/pkg/tagger/types v0.67.0
	github.com/DataDog/datadog-agent/pkg/tagset v0.67.0
	github.com/DataDog/datadog-agent/pkg/telemetry v0.67.0
	github.com/DataDog/datadog-agent/pkg/template v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/cache v0.61.0
	github.com/DataDog/datadog-agent/pkg/util/common v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/containers/image v0.56.2
	github.com/DataDog/datadog-agent/pkg/util/executable v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/flavor v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/grpc v0.60.0
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/http v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/json v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/log/setup v0.62.2
	github.com/DataDog/datadog-agent/pkg/util/option v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/sort v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/startstop v0.64.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/system v0.67.0
	github.com/DataDog/datadog-agent/pkg/util/testutil v0.59.0
	github.com/DataDog/datadog-agent/pkg/util/uuid v0.59.0
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.67.0
	github.com/DataDog/datadog-agent/pkg/version v0.67.0
	github.com/DataDog/go-libddwaf/v3 v3.5.4
	github.com/DataDog/go-sqllexer v0.1.6
	github.com/Datadog/dublin-traceroute v0.0.2
	github.com/aquasecurity/trivy v0.49.2-0.20240227072422-e1ea02c7b80d
	github.com/aws/aws-sdk-go-v2/service/kms v1.38.1
	github.com/aws/aws-sdk-go-v2/service/rds v1.94.2
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.35.2
	github.com/cloudfoundry-community/go-cfclient/v2 v2.0.1-0.20230503155151-3d15366c5820
	github.com/containerd/cgroups/v3 v3.0.5
	github.com/containerd/typeurl/v2 v2.2.3
	github.com/elastic/go-seccomp-bpf v1.5.0
	github.com/fatih/structtag v1.2.0
	github.com/glaslos/ssdeep v0.4.0
	github.com/gocomply/scap v0.1.2-0.20230531064509-55a00f73e8d6
	github.com/godror/godror v0.37.0
	github.com/google/cel-go v0.25.0
	github.com/gorilla/handlers v1.5.2
	github.com/jackc/pgx/v5 v5.6.0
	github.com/jmoiron/sqlx v1.4.0
	github.com/judwhite/go-svc v1.2.1
	github.com/kr/pretty v0.3.1
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
	github.com/prometheus-community/pro-bing v0.4.1
	github.com/rickar/props v1.0.0
	github.com/sijms/go-ora/v2 v2.8.24
	github.com/swaggest/jsonschema-go v0.3.70
	github.com/valyala/fastjson v1.6.4
	github.com/vibrantbyte/go-antpath v1.1.1
	go.opentelemetry.io/collector/confmap/provider/envprovider v1.35.0
	go.opentelemetry.io/collector/confmap/provider/fileprovider v1.35.0
	go.opentelemetry.io/collector/confmap/provider/httpprovider v1.35.0
	go.opentelemetry.io/collector/confmap/provider/httpsprovider v1.35.0
	go.opentelemetry.io/collector/confmap/provider/yamlprovider v1.35.0
	go.opentelemetry.io/collector/extension v1.35.0
	go.opentelemetry.io/collector/otelcol v0.129.0
	go.opentelemetry.io/collector/processor v1.35.0
	go.opentelemetry.io/collector/service v0.129.0
	go4.org/intern v0.0.0-20230525184215-6c62f75575cb
	go4.org/mem v0.0.0-20220726221520-4f986261bf13
	k8s.io/cli-runtime v0.31.2
	k8s.io/kubectl v0.31.2
)

require go.opentelemetry.io/collector/pdata/xpdata v0.129.0 // indirect

require (
	github.com/prometheus/otlptranslator v0.0.0-20250320144820-d800c8b0eb07 // indirect
	go.opentelemetry.io/collector/config/configoptional v0.129.0 // indirect
)

require (
	github.com/aws/aws-sdk-go-v2/service/ecs v1.58.0 // indirect
	github.com/gophercloud/gophercloud/v2 v2.7.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/gopsutilenv v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatocumulativeprocessor v0.129.0 // indirect
	github.com/prometheus/sigv4 v0.1.2 // indirect
)

require (
	github.com/foxboron/go-tpm-keyfiles v0.0.0-20250323135004-b31fac66206e // indirect
	github.com/google/go-tpm v0.9.5 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/winperfcounters v0.129.0 // indirect
)

require (
	github.com/moby/sys/atomicwriter v0.1.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/routingconnector v0.129.0 // indirect
	go.opentelemetry.io/collector/config/configmiddleware v0.129.0 // indirect
	go.opentelemetry.io/collector/extension/extensionmiddleware v0.129.0 // indirect
)

require (
	cel.dev/expr v0.23.1 // indirect
	cloud.google.com/go/auth v0.16.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute v1.37.0 // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20240604201846-c756bfed2ed3 // indirect
	code.cloudfoundry.org/go-loggregator v7.4.0+incompatible // indirect
	code.cloudfoundry.org/rfc5424 v0.0.0-20201103192249-000122071b78 // indirect
	dario.cat/mergo v1.0.1 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20240806141605-e8a1dd7889d6 // indirect
	github.com/AdamKorcz/go-118-fuzz-build v0.0.0-20231105174938-2b5cbb29f3e2 // indirect
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.18.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.9.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.3.0 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.29 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.23 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.4.2 // indirect
	github.com/Code-Hex/go-generics-cache v1.5.1 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/statsprocessor v0.64.0-rc.12 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/viperconfig v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/buf v0.67.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/statstracker v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.67.0 // indirect
	github.com/DataDog/datadog-api-client-go/v2 v2.41.0 // indirect
	github.com/DataDog/go-runtime-metrics-internal v0.0.4-0.20241206090539-a14610dc22b6 // indirect
	github.com/DataDog/gohai v0.0.0-20230524154621-4316413895ee // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/inframetadata v0.29.0 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/logs v0.29.0 // indirect
	github.com/GoogleCloudPlatform/docker-credential-gcr v2.0.5+incompatible // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.27.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Showmax/go-fqdn v1.0.0 // indirect
	github.com/alecthomas/participle/v2 v2.1.4 // indirect
	github.com/antchfx/xmlquery v1.4.4 // indirect
	github.com/antchfx/xpath v1.3.4 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/apache/thrift v0.22.0 // indirect
	github.com/aquasecurity/go-gem-version v0.0.0-20201115065557-8eed6fe000ce // indirect
	github.com/aquasecurity/go-npm-version v0.0.1 // indirect
	github.com/aquasecurity/go-pep440-version v0.0.1 // indirect
	github.com/aquasecurity/go-version v0.0.1 // indirect
	github.com/aquasecurity/jfather v0.0.8 // indirect
	github.com/aquasecurity/trivy-java-db v0.0.0-20240109071736-184bd7481d48 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.42.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/servicediscovery v1.35.7 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/bhmj/xpression v0.9.1 // indirect
	github.com/bitnami/go-version v0.0.0-20231130084017-bb00604d650c // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/bmatcuk/doublestar/v4 v4.8.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/chrusty/protoc-gen-jsonschema v0.0.0-20240212064413-73d5723042b8 // indirect
	github.com/cloudflare/circl v1.6.1 // indirect
	github.com/cncf/xds/go v0.0.0-20250326154945-ae57f3c0d45f // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/platforms v1.0.0-rc.1 // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/digitalocean/godo v1.144.0 // indirect
	github.com/eapache/queue/v2 v2.0.0-20230407133247-75960ed334e4 // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/elastic/go-grok v0.3.1 // indirect
	github.com/elastic/go-licenser v0.4.2 // indirect
	github.com/elastic/lunes v0.1.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.1 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/glebarez/go-sqlite v1.22.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20250517221953-25912455fbc8
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-resty/resty/v2 v2.16.5 // indirect
	github.com/go-test/deep v1.1.0 // indirect
	github.com/go-zookeeper/zk v1.0.4 // indirect
	github.com/gobuffalo/flect v1.0.2 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.11.0 // indirect
	github.com/godror/knownpb v0.1.0 // indirect
	github.com/gofrs/flock v0.12.1 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/gopherjs/gopherjs v0.0.0-20200217142428-fce0ec30dd00 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/grafana/regexp v0.0.0-20240518133315-a468a5bfb3bc // indirect
	github.com/hashicorp/cronexpr v1.1.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.7 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.8 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/nomad/api v0.0.0-20241218080744-e3ac00f30eec // indirect
	github.com/hetznercloud/hcloud-go/v2 v2.21.0 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/ionos-cloud/sdk-go/v6 v6.3.3 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jaegertracing/jaeger v1.66.0 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.0 // indirect
	github.com/knadh/koanf/v2 v2.2.1 // indirect
	github.com/knqyf263/go-apk-version v0.0.0-20200609155635-041fdbb8563f // indirect
	github.com/knqyf263/nested v0.0.1 // indirect
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/leodido/go-syslog/v4 v4.2.0 // indirect
	github.com/leodido/ragel-machinery v0.0.0-20190525184631-5f46317e436b // indirect
	github.com/lightstep/go-expohisto v1.0.0 // indirect
	github.com/linode/linodego v1.49.0 // indirect
	github.com/lunixbochs/struc v0.0.0-20200707160740-784aaebc1d40 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/masahiro331/go-mvn-version v0.0.0-20250131095131-f4974fa13b8a // indirect
	github.com/mattn/go-shellwords v1.0.12 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/spdystream v0.5.0 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/oklog/ulid/v2 v2.1.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/sapmexporter v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/dockerobserver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/ecsobserver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/ecstaskobserver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/hostobserver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/k8sobserver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/aws/ecsutil v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/common v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/docker v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/exp/metrics v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/k8sconfig v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/metadataproviders v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/pdatautil v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/sharedcomponent v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/splunk v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/batchperresourceattr v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/batchpersignal v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/experimentalmetricmetadata v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/sampling v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/zipkin v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/cumulativetodeltaprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/filterprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/groupbyattrsprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/probabilisticsamplerprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/routingprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/fluentforwardreceiver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/receivercreator v0.129.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver v0.129.0 // indirect
	github.com/openshift/client-go v0.0.0-20241203091221-452dfb8fa071 // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/ovh/go-ovh v1.7.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/prometheus/prometheus v0.304.1 // indirect
	github.com/puzpuzpuz/xsync/v3 v3.5.1 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/rust-secure-code/go-rustaudit v0.0.0-20250226111315-e20ec32e963c // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/samber/oops v1.15.0 // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.33 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/signalfx/sapm-proto v0.17.0 // indirect
	github.com/smartystreets/assertions v1.1.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/stormcat24/protodep v0.1.8 // indirect
	github.com/swaggest/refl v1.3.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/ua-parser/uap-go v0.0.0-20240611065828-3a4781585db6 // indirect
	github.com/vultr/govultr/v2 v2.17.2 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/collector/client v1.35.0 // indirect
	go.opentelemetry.io/collector/component/componentstatus v0.129.0 // indirect
	go.opentelemetry.io/collector/config/configauth v0.129.0 // indirect
	go.opentelemetry.io/collector/config/configcompression v1.35.0 // indirect
	go.opentelemetry.io/collector/config/configgrpc v0.129.0 // indirect
	go.opentelemetry.io/collector/config/confighttp v0.129.0 // indirect
	go.opentelemetry.io/collector/config/confignet v1.35.0 // indirect
	go.opentelemetry.io/collector/config/configopaque v1.35.0 // indirect
	go.opentelemetry.io/collector/config/configretry v1.35.0 // indirect
	go.opentelemetry.io/collector/config/configtls v1.35.0 // indirect
	go.opentelemetry.io/collector/connector v0.129.0 // indirect
	go.opentelemetry.io/collector/consumer/consumertest v0.129.0 // indirect
	go.opentelemetry.io/collector/exporter/nopexporter v0.129.0 // indirect
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.129.0 // indirect
	go.opentelemetry.io/collector/extension/extensioncapabilities v0.129.0 // indirect
	go.opentelemetry.io/collector/extension/zpagesextension v0.129.0 // indirect
	go.opentelemetry.io/collector/filter v0.129.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.129.0 // indirect
	go.opentelemetry.io/collector/pdata/testdata v0.129.0 // indirect
	go.opentelemetry.io/collector/pipeline v0.129.0 // indirect
	go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.129.0 // indirect
	go.opentelemetry.io/collector/receiver/nopreceiver v0.129.0 // indirect
	go.opentelemetry.io/collector/semconv v0.128.1-0.20250610090210-188191247685 // indirect
	go.opentelemetry.io/contrib/zpages v0.61.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.12.2 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.36.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.36.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.12.2 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.36.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.36.0 // indirect
	go.opentelemetry.io/otel/log v0.12.2 // indirect
	go.opentelemetry.io/otel/sdk/log v0.12.2 // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20231121144256-b99613f794b6 // indirect
	golang.org/x/exp/typeparams v0.0.0-20240314144324-c7f7c6466f7f // indirect
	golang.org/x/lint v0.0.0-20241112194109-818c5a804067 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250519155744-55703ea1f237 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250603155806-513f23925822 // indirect
	gopkg.in/cheggaaa/pb.v1 v1.0.28 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	honnef.co/go/tools v0.5.1 // indirect
	k8s.io/kms v0.32.1 // indirect
	k8s.io/sample-controller v0.31.2 // indirect
)

// TODO(songy23): remove this once https://github.com/kubernetes/apiserver/commit/b887c9ebecf558a2001fc5c5dbd5c87fd672500c is brought to agent
replace go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.60.0

replace github.com/pahanini/go-grpc-bidirectional-streaming-example v0.0.0-20211027164128-cc6111af44be => github.com/DataDog/go-grpc-bidirectional-streaming-example v0.0.0-20221024060302-b9cf785c02fe

replace github.com/vishvananda/netlink => github.com/DataDog/netlink v1.0.1-0.20240223195320-c7a4f832a3d1

// Use custom Trivy fork to reduce binary size
// Pull in replacements needed by upstream Trivy
replace (
	// Maps to Trivy fork https://github.com/DataDog/trivy/commits/djc/main-dd-060
	github.com/aquasecurity/trivy => github.com/DataDog/trivy v0.0.0-20250328083658-40561b7842af
	github.com/saracen/walker => github.com/DataDog/walker v0.0.0-20230418153152-7f29bb2dc950
)

// Prevent dependencies to be bumped by Trivy
// github.com/DataDog/aptly@v1.5.3 depends on gopenpgp/v2, so we use latest version of go-crypto before the move to gopenpgp/v3
replace github.com/ProtonMail/go-crypto => github.com/ProtonMail/go-crypto v1.0.0

// Prevent a false-positive detection by the Google and Ikarus security vendors on VirusTotal
exclude go.opentelemetry.io/proto/otlp v1.1.0

replace github.com/google/gopacket v1.1.19 => github.com/DataDog/gopacket v0.0.0-20250206221735-64e5a8c92d94

// Remove once https://github.com/kubernetes/kube-state-metrics/pull/2553 is merged
replace k8s.io/kube-state-metrics/v2 v2.13.1-0.20241025121156-110f03d7331f => github.com/L3n41c/kube-state-metrics/v2 v2.13.1-0.20241119155242-07761b9fe9a0

// Remove once https://github.com/Iceber/iouring-go/pull/31 or equivalent is merged,
// among with the Connect, Bind and Accept requests
replace github.com/iceber/iouring-go => github.com/lebauce/iouring-go v0.0.0-20250513121434-2d4fb49003b5

// github.com/golang/mock is unmaintained and archived, v1.6.0 is the last released version
replace github.com/golang/mock => github.com/golang/mock v1.6.0

// Remove once sigs.k8s.io/custom-metrics-apiserver upgrades to k8s.io/* v0.32+
replace (
	k8s.io/api => k8s.io/api v0.31.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.31.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.31.2
	k8s.io/apiserver => k8s.io/apiserver v0.31.2
	k8s.io/client-go => k8s.io/client-go v0.31.2
)

// Fork to remove some text/template usage, https://github.com/paulcacheux/opa/tree/lightweight
replace github.com/open-policy-agent/opa => github.com/paulcacheux/opa v0.0.0-20250507091703-12dadb609ad1

// This section was automatically added by 'dda inv modules.add-all-replace' command, do not edit manually

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ./comp/api/api/def
	github.com/DataDog/datadog-agent/comp/core/config => ./comp/core/config
	github.com/DataDog/datadog-agent/comp/core/configsync => ./comp/core/configsync
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ./comp/core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ./comp/core/flare/types
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface => ./comp/core/hostname/hostnameinterface
	github.com/DataDog/datadog-agent/comp/core/ipc/def => ./comp/core/ipc/def
	github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers => ./comp/core/ipc/httphelpers
	github.com/DataDog/datadog-agent/comp/core/ipc/impl => ./comp/core/ipc/impl
	github.com/DataDog/datadog-agent/comp/core/ipc/mock => ./comp/core/ipc/mock
	github.com/DataDog/datadog-agent/comp/core/log/def => ./comp/core/log/def
	github.com/DataDog/datadog-agent/comp/core/log/fx => ./comp/core/log/fx
	github.com/DataDog/datadog-agent/comp/core/log/impl => ./comp/core/log/impl
	github.com/DataDog/datadog-agent/comp/core/log/impl-trace => ./comp/core/log/impl-trace
	github.com/DataDog/datadog-agent/comp/core/log/mock => ./comp/core/log/mock
	github.com/DataDog/datadog-agent/comp/core/secrets => ./comp/core/secrets
	github.com/DataDog/datadog-agent/comp/core/status => ./comp/core/status
	github.com/DataDog/datadog-agent/comp/core/status/statusimpl => ./comp/core/status/statusimpl
	github.com/DataDog/datadog-agent/comp/core/tagger/def => ./comp/core/tagger/def
	github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote => ./comp/core/tagger/fx-remote
	github.com/DataDog/datadog-agent/comp/core/tagger/generic_store => ./comp/core/tagger/generic_store
	github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote => ./comp/core/tagger/impl-remote
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection => ./comp/core/tagger/origindetection
	github.com/DataDog/datadog-agent/comp/core/tagger/subscriber => ./comp/core/tagger/subscriber
	github.com/DataDog/datadog-agent/comp/core/tagger/tags => ./comp/core/tagger/tags
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry => ./comp/core/tagger/telemetry
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ./comp/core/tagger/types
	github.com/DataDog/datadog-agent/comp/core/tagger/utils => ./comp/core/tagger/utils
	github.com/DataDog/datadog-agent/comp/core/telemetry => ./comp/core/telemetry
	github.com/DataDog/datadog-agent/comp/def => ./comp/def
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ./comp/forwarder/defaultforwarder
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface => ./comp/forwarder/orchestrator/orchestratorinterface
	github.com/DataDog/datadog-agent/comp/logs/agent/config => ./comp/logs/agent/config
	github.com/DataDog/datadog-agent/comp/netflow/payload => ./comp/netflow/payload
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def => ./comp/otelcol/collector-contrib/def
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl => ./comp/otelcol/collector-contrib/impl
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def => ./comp/otelcol/converter/def
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl => ./comp/otelcol/converter/impl
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def => ./comp/otelcol/ddflareextension/def
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl => ./comp/otelcol/ddflareextension/impl
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types => ./comp/otelcol/ddflareextension/types
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def => ./comp/otelcol/ddprofilingextension/def
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl => ./comp/otelcol/ddprofilingextension/impl
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline => ./comp/otelcol/logsagentpipeline
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl => ./comp/otelcol/logsagentpipeline/logsagentpipelineimpl
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/connector/datadogconnector => ./comp/otelcol/otlp/components/connector/datadogconnector
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter => ./comp/otelcol/otlp/components/exporter/datadogexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter => ./comp/otelcol/otlp/components/exporter/logsagentexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter => ./comp/otelcol/otlp/components/exporter/serializerexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient => ./comp/otelcol/otlp/components/metricsclient
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor => ./comp/otelcol/otlp/components/processor/infraattributesprocessor
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/statsprocessor => ./comp/otelcol/otlp/components/statsprocessor
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil => ./comp/otelcol/otlp/testutil
	github.com/DataDog/datadog-agent/comp/otelcol/status/def => ./comp/otelcol/status/def
	github.com/DataDog/datadog-agent/comp/otelcol/status/impl => ./comp/otelcol/status/impl
	github.com/DataDog/datadog-agent/comp/serializer/logscompression => ./comp/serializer/logscompression
	github.com/DataDog/datadog-agent/comp/serializer/metricscompression => ./comp/serializer/metricscompression
	github.com/DataDog/datadog-agent/comp/trace/agent/def => ./comp/trace/agent/def
	github.com/DataDog/datadog-agent/comp/trace/compression/def => ./comp/trace/compression/def
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip => ./comp/trace/compression/impl-gzip
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd => ./comp/trace/compression/impl-zstd
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ./pkg/aggregator/ckey
	github.com/DataDog/datadog-agent/pkg/api => ./pkg/api
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ./pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/create => ./pkg/config/create
	github.com/DataDog/datadog-agent/pkg/config/env => ./pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/mock => ./pkg/config/mock
	github.com/DataDog/datadog-agent/pkg/config/model => ./pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ./pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/remote => ./pkg/config/remote
	github.com/DataDog/datadog-agent/pkg/config/setup => ./pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/structure => ./pkg/config/structure
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ./pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/config/utils => ./pkg/config/utils
	github.com/DataDog/datadog-agent/pkg/config/viperconfig => ./pkg/config/viperconfig
	github.com/DataDog/datadog-agent/pkg/errors => ./pkg/errors
	github.com/DataDog/datadog-agent/pkg/fips => ./pkg/fips
	github.com/DataDog/datadog-agent/pkg/fleet/installer => ./pkg/fleet/installer
	github.com/DataDog/datadog-agent/pkg/gohai => ./pkg/gohai
	github.com/DataDog/datadog-agent/pkg/linters/components/pkgconfigusage => ./pkg/linters/components/pkgconfigusage
	github.com/DataDog/datadog-agent/pkg/logs/client => ./pkg/logs/client
	github.com/DataDog/datadog-agent/pkg/logs/diagnostic => ./pkg/logs/diagnostic
	github.com/DataDog/datadog-agent/pkg/logs/message => ./pkg/logs/message
	github.com/DataDog/datadog-agent/pkg/logs/metrics => ./pkg/logs/metrics
	github.com/DataDog/datadog-agent/pkg/logs/pipeline => ./pkg/logs/pipeline
	github.com/DataDog/datadog-agent/pkg/logs/processor => ./pkg/logs/processor
	github.com/DataDog/datadog-agent/pkg/logs/sds => ./pkg/logs/sds
	github.com/DataDog/datadog-agent/pkg/logs/sender => ./pkg/logs/sender
	github.com/DataDog/datadog-agent/pkg/logs/sources => ./pkg/logs/sources
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface => ./pkg/logs/status/statusinterface
	github.com/DataDog/datadog-agent/pkg/logs/status/utils => ./pkg/logs/status/utils
	github.com/DataDog/datadog-agent/pkg/logs/util/testutils => ./pkg/logs/util/testutils
	github.com/DataDog/datadog-agent/pkg/metrics => ./pkg/metrics
	github.com/DataDog/datadog-agent/pkg/network/payload => ./pkg/network/payload
	github.com/DataDog/datadog-agent/pkg/networkdevice/profile => ./pkg/networkdevice/profile
	github.com/DataDog/datadog-agent/pkg/networkpath/payload => ./pkg/networkpath/payload
	github.com/DataDog/datadog-agent/pkg/obfuscate => ./pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata => ./pkg/opentelemetry-mapping-go/inframetadata
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/gohai/internal/gohaitest => ./pkg/opentelemetry-mapping-go/inframetadata/gohai/internal/gohaitest
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes => ./pkg/opentelemetry-mapping-go/otlp/attributes
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/logs => ./pkg/opentelemetry-mapping-go/otlp/logs
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics => ./pkg/opentelemetry-mapping-go/otlp/metrics
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ./pkg/orchestrator/model
	github.com/DataDog/datadog-agent/pkg/process/util/api => ./pkg/process/util/api
	github.com/DataDog/datadog-agent/pkg/proto => ./pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ./pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/security/secl => ./pkg/security/secl
	github.com/DataDog/datadog-agent/pkg/security/seclwin => ./pkg/security/seclwin
	github.com/DataDog/datadog-agent/pkg/serializer => ./pkg/serializer
	github.com/DataDog/datadog-agent/pkg/status/health => ./pkg/status/health
	github.com/DataDog/datadog-agent/pkg/tagger/types => ./pkg/tagger/types
	github.com/DataDog/datadog-agent/pkg/tagset => ./pkg/tagset
	github.com/DataDog/datadog-agent/pkg/telemetry => ./pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/template => ./pkg/template
	github.com/DataDog/datadog-agent/pkg/trace => ./pkg/trace
	github.com/DataDog/datadog-agent/pkg/trace/stats/oteltest => ./pkg/trace/stats/oteltest
	github.com/DataDog/datadog-agent/pkg/util/backoff => ./pkg/util/backoff
	github.com/DataDog/datadog-agent/pkg/util/buf => ./pkg/util/buf
	github.com/DataDog/datadog-agent/pkg/util/cache => ./pkg/util/cache
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ./pkg/util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/common => ./pkg/util/common
	github.com/DataDog/datadog-agent/pkg/util/compression => ./pkg/util/compression
	github.com/DataDog/datadog-agent/pkg/util/containers/image => ./pkg/util/containers/image
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths => ./pkg/util/defaultpaths
	github.com/DataDog/datadog-agent/pkg/util/executable => ./pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ./pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/flavor => ./pkg/util/flavor
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ./pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/grpc => ./pkg/util/grpc
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ./pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/http => ./pkg/util/http
	github.com/DataDog/datadog-agent/pkg/util/json => ./pkg/util/json
	github.com/DataDog/datadog-agent/pkg/util/log => ./pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/log/setup => ./pkg/util/log/setup
	github.com/DataDog/datadog-agent/pkg/util/option => ./pkg/util/option
	github.com/DataDog/datadog-agent/pkg/util/otel => ./pkg/util/otel
	github.com/DataDog/datadog-agent/pkg/util/pointer => ./pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/prometheus => ./pkg/util/prometheus
	github.com/DataDog/datadog-agent/pkg/util/quantile => ./pkg/util/quantile
	github.com/DataDog/datadog-agent/pkg/util/quantile/sketchtest => ./pkg/util/quantile/sketchtest
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ./pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/sort => ./pkg/util/sort
	github.com/DataDog/datadog-agent/pkg/util/startstop => ./pkg/util/startstop
	github.com/DataDog/datadog-agent/pkg/util/statstracker => ./pkg/util/statstracker
	github.com/DataDog/datadog-agent/pkg/util/system => ./pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ./pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ./pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/utilizationtracker => ./pkg/util/utilizationtracker
	github.com/DataDog/datadog-agent/pkg/util/uuid => ./pkg/util/uuid
	github.com/DataDog/datadog-agent/pkg/util/winutil => ./pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ./pkg/version
	github.com/DataDog/datadog-agent/test/fakeintake => ./test/fakeintake
	github.com/DataDog/datadog-agent/test/new-e2e => ./test/new-e2e
	github.com/DataDog/datadog-agent/test/otel => ./test/otel

)
