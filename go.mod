module github.com/DataDog/datadog-agent

go 1.16

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
	github.com/docker/distribution => github.com/docker/distribution v2.7.1-0.20190104202606-0ac367fd6bee+incompatible
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	github.com/lxn/walk => github.com/lxn/walk v0.0.0-20180521183810-02935bac0ab8
	github.com/mholt/archiver => github.com/mholt/archiver v2.0.1-0.20171012052341-26cf5bb32d07+incompatible
	github.com/spf13/cast => github.com/DataDog/cast v1.3.1-0.20190301154711-1ee8c8bd14a3
	github.com/ugorji/go => github.com/ugorji/go v1.1.7
)

// pinned to grpc v1.28.0
replace (
	github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.12.2
	google.golang.org/grpc => github.com/grpc/grpc-go v1.28.0
)

replace (
	github.com/DataDog/datadog-agent/pkg/obfuscate => ./pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/otlp/model => ./pkg/otlp/model
	github.com/DataDog/datadog-agent/pkg/quantile => ./pkg/quantile
	github.com/DataDog/datadog-agent/pkg/security/secl => ./pkg/security/secl
	github.com/DataDog/datadog-agent/pkg/util/log => ./pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ./pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/winutil => ./pkg/util/winutil
)

require (
	code.cloudfoundry.org/bbs v0.0.0-20200403215808-d7bc971db0db
	code.cloudfoundry.org/cfhttp/v2 v2.0.0 // indirect
	code.cloudfoundry.org/clock v1.0.0 // indirect
	code.cloudfoundry.org/consuladapter v0.0.0-20200131002136-ac1daf48ba97 // indirect
	code.cloudfoundry.org/diego-logging-client v0.0.0-20200130234554-60ef08820a45 // indirect
	code.cloudfoundry.org/executor v0.0.0-20200218194701-024d0bdd52d4 // indirect
	code.cloudfoundry.org/garden v0.0.0-20210208153517-580cadd489d2
	code.cloudfoundry.org/go-diodes v0.0.0-20190809170250-f77fb823c7ee // indirect
	code.cloudfoundry.org/go-loggregator v7.4.0+incompatible // indirect
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/locket v0.0.0-20200131001124-67fd0a0fdf2d // indirect
	code.cloudfoundry.org/rep v0.0.0-20200325195957-1404b978e31e // indirect
	code.cloudfoundry.org/rfc5424 v0.0.0-20180905210152-236a6d29298a // indirect
	code.cloudfoundry.org/tlsconfig v0.0.0-20200131000646-bbe0f8da39b3 // indirect
	github.com/BurntSushi/toml v0.4.1 // indirect
	github.com/DataDog/agent-payload/v5 v5.0.4
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/otlp/model v0.33.0-rc.4
	github.com/DataDog/datadog-agent/pkg/quantile v0.33.0-rc.4
	github.com/DataDog/datadog-agent/pkg/security/secl v0.33.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/log v0.33.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.33.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.33.0-rc.4
	github.com/DataDog/datadog-go v4.8.2+incompatible
	github.com/DataDog/datadog-operator v0.5.0-rc.2.0.20210402083916-25ba9a22e67a
	github.com/DataDog/ebpf v0.0.0-20211116165855-af5870810f0b
	github.com/DataDog/ebpf-manager v0.0.0-20211116173716-a65628f678af
	github.com/DataDog/gohai v0.0.0-20211126091652-d183ed971098
	github.com/DataDog/gopsutil v0.0.0-20211112180027-9aa392ae181a
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/nikos v1.6.2
	github.com/DataDog/sketches-go v1.2.1
	github.com/DataDog/viper v1.9.0
	github.com/DataDog/watermarkpodautoscaler v0.3.1-logs-attributes.2.0.20211014120627-6d6a5c559fc9
	github.com/DataDog/zstd v1.4.8
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/Microsoft/go-winio v0.5.1
	github.com/Microsoft/hcsshim v0.9.0
	github.com/acobaugh/osrelease v0.1.0
	github.com/alecthomas/jsonschema v0.0.0-20210526225647-edb03dcab7bc
	github.com/alecthomas/participle v0.7.1
	github.com/alecthomas/repr v0.0.0-20181024024818-d37bc2a10ba1
	github.com/andybalholm/brotli v1.0.1 // indirect
	github.com/avast/retry-go v3.0.0+incompatible
	github.com/aws/aws-sdk-go v1.42.20
	github.com/beevik/ntp v0.3.0
	github.com/benbjohnson/clock v1.1.0
	github.com/benesch/cgosymbolizer v0.0.0-20190515212042-bec6fe6e597b
	github.com/bhmj/jsonslice v0.0.0-20200323023432-92c3edaad8e2
	github.com/blabber/go-freebsd-sysctl v0.0.0-20201130114544-503969f39d8f
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/cilium/ebpf v0.6.3-0.20210917122031-fc2955d2ecee
	github.com/clbanning/mxj v1.8.4
	github.com/cloudfoundry-community/go-cfclient v0.0.0-20210621174645-7773f7e22665
	github.com/containerd/cgroups v1.0.2
	github.com/containerd/containerd v1.5.7
	github.com/containerd/typeurl v1.0.2
	github.com/coreos/go-semver v0.3.0
	github.com/coreos/go-systemd v0.0.0-20190620071333-e64a0ec8b42a
	github.com/cri-o/ocicni v0.2.0
	github.com/cyphar/filepath-securejoin v0.2.2
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/docker v17.12.0-ce-rc1.0.20200916142827-bd33bbf0497b+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/dustin/go-humanize v1.0.0
	github.com/elastic/go-libaudit v0.4.0
	github.com/fatih/color v1.13.0
	github.com/florianl/go-conntrack v0.2.0
	github.com/freddierice/go-losetup v0.0.0-20170407175016-fc9adea44124
	github.com/go-ini/ini v1.63.2
	github.com/go-ole/go-ole v1.2.5
	github.com/go-openapi/spec v0.20.4
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/go-test/deep v1.0.5 // indirect
	github.com/gobwas/glob v0.2.3
	github.com/godbus/dbus v4.1.0+incompatible
	github.com/gogo/protobuf v1.3.2
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/google/gofuzz v1.2.0
	github.com/google/gopacket v1.1.19
	github.com/google/pprof v0.0.0-20210423192551-a2663126120b
	github.com/gorilla/mux v1.8.0
	github.com/gosnmp/gosnmp v1.32.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/grpc-ecosystem/grpc-gateway v1.16.0
	github.com/h2non/filetype v1.1.2-0.20210602110014-3305bbb7ac7b
	github.com/hashicorp/consul/api v1.11.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru v0.5.4
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/ianlancetaylor/cgosymbolizer v0.0.0-20201204192058-7acc97e53614 // indirect
	github.com/iceber/iouring-go v0.0.0-20210726032807-b073cc83b2b8
	github.com/imdario/mergo v0.3.12
	github.com/iovisor/gobpf v0.2.0
	github.com/itchyny/gojq v0.12.5
	github.com/json-iterator/go v1.1.12
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/karrick/godirwalk v1.16.1
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/kubernetes-sigs/custom-metrics-apiserver v0.0.0-20210311094424-0ca2b1909cdc
	github.com/lib/pq v1.10.0 // indirect
	github.com/lxn/walk v0.0.0-20191128110447-55ccb3a9f5c1
	github.com/lxn/win v0.0.0-20191128105842-2da648fda5b4
	github.com/mailru/easyjson v0.7.7
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mdlayher/netlink v1.4.1
	github.com/mholt/archiver/v3 v3.5.0
	github.com/miekg/dns v1.1.43
	github.com/mitchellh/mapstructure v1.4.2
	github.com/moby/sys/mountinfo v0.4.1
	github.com/mohae/deepcopy v0.0.0-20170603005431-491d3605edfb
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/olekukonko/tablewriter v0.0.5
	github.com/oliveagle/jsonpath v0.0.0-20180606110733-2e52cf6e6852 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.17.0 // indirect
	github.com/open-policy-agent/opa v0.35.0
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.38.0
	github.com/opencontainers/image-spec v1.0.2-0.20210819154149-5ad6f50d6283 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417
	github.com/opencontainers/selinux v1.9.1 // indirect
	github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pierrec/lz4/v4 v4.1.3 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/richardartoul/molecule v0.0.0-20210914193524-25d8911bb85b
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da
	github.com/shirou/gopsutil v3.21.9+incompatible
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4
	github.com/shuLhan/go-bindata v4.0.0+incompatible
	github.com/spf13/afero v1.6.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00 // indirect
	github.com/tent/canonical-json-go v0.0.0-20130607151641-96e4ba3a7613
	github.com/theupdateframework/go-tuf v0.0.0-20210921152604-1c7bbcecec00
	github.com/tinylib/msgp v1.1.6
	github.com/twmb/murmur3 v1.1.6
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/urfave/negroni v1.0.0
	github.com/vishvananda/netlink v1.1.1-0.20210508154835-66ddd91f7ddd
	github.com/vishvananda/netns v0.0.0-20210104183010-2eb08e3e575f
	github.com/vito/go-sse v1.0.0 // indirect
	github.com/vmihailenco/msgpack/v4 v4.3.12
	github.com/xeipuuv/gojsonschema v1.2.0
	go.etcd.io/bbolt v1.3.6
	go.etcd.io/etcd/client/v2 v2.305.1
	go.opentelemetry.io/collector v0.38.0
	go.opentelemetry.io/collector/model v0.38.0
	// Fix vanity import issue
	go.opentelemetry.io/otel/internal/metric v0.24.1-0.20211006140346-3d4ae8d0b75f // indirect
	go.uber.org/automaxprocs v1.4.0
	go.uber.org/multierr v1.7.0
	go.uber.org/zap v1.19.1
	go4.org/intern v0.0.0-20210108033219-3eb7198706b2
	golang.org/x/mobile v0.0.0-20201217150744-e6ae53a27f4f
	golang.org/x/net v0.0.0-20211111083644-e5c967477495
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211110154304-99a53858aa08
	golang.org/x/text v0.3.7
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	golang.org/x/tools v0.1.8
	gomodules.xyz/jsonpatch/v3 v3.0.1
	google.golang.org/genproto v0.0.0-20210604141403-392c879c8b08
	google.golang.org/grpc v1.41.0
	gopkg.in/DataDog/dd-trace-go.v1 v1.34.0
	gopkg.in/Knetic/govaluate.v3 v3.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.21.5
	k8s.io/apimachinery v0.21.5
	k8s.io/apiserver v0.21.5
	k8s.io/autoscaler/vertical-pod-autoscaler v0.9.2
	k8s.io/client-go v0.21.5
	k8s.io/cri-api v0.21.5
	k8s.io/klog v1.0.1-0.20200310124935-4ad0115ba9e4 // Min version that includes fix for Windows Nano
	k8s.io/klog/v2 v2.9.0
	k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
	k8s.io/kube-state-metrics/v2 v2.1.1
	k8s.io/metrics v0.21.5
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
)

// Fixing a CVE on a transitive dep of k8s/etcd, should be cleaned-up once k8s.io/apiserver dep is removed (but double-check with `go mod why` that no other dep pulls it)
replace github.com/dgrijalva/jwt-go => github.com/golang-jwt/jwt v3.2.1+incompatible

// Include bug fixes not released upstream (yet)
// - https://github.com/kubernetes/kube-state-metrics/pull/1610
// - https://github.com/kubernetes/kube-state-metrics/pull/1584
replace k8s.io/kube-state-metrics/v2 => github.com/DataDog/kube-state-metrics/v2 v2.1.2-0.20211109105526-c17162ee2798

// Exclude this version of containerd because it depends on github.com/Microsoft/hcsshim@v0.8.7 which depends on k8s.io/kubernetes which is a dependency we’d like to avoid
exclude github.com/containerd/containerd v1.5.0-beta.1

// Remove once the issue https://github.com/microsoft/Windows-Containers/issues/72 is resolved
replace github.com/golang/glog v1.0.0 => github.com/paulcacheux/glog v1.0.1-0.20211019114809-ec0f43a655b9
