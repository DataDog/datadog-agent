module github.com/DataDog/datadog-agent

go 1.17

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

// pinned to grpc v1.28.0 - this pin is required due to k8s.io/apiserver (and other) pins being set to v0.21.5
//                          the kubernetes pins are required at this time. We will bump grpc once k8s releases
//                          v1.24 or v1.25. Keep in sync with the version in internal/patch/grpc-go-insecure.
replace (
	github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.12.2
	google.golang.org/grpc => github.com/grpc/grpc-go v1.28.0
)

// HACK: Add `insecure` package (added on grpc-go v1.34) to support packages using it (notably go.opentelemetry/collector)
// See internal/patch/grpc-go-insecure/README.md for more details.
replace google.golang.org/grpc/credentials/insecure => ./internal/patch/grpc-go-insecure

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
	code.cloudfoundry.org/garden v0.0.0-20210208153517-580cadd489d2
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/DataDog/agent-payload/v5 v5.0.10
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.34.0-rc.4
	github.com/DataDog/datadog-agent/pkg/otlp/model v0.34.0-rc.4
	github.com/DataDog/datadog-agent/pkg/quantile v0.34.0-rc.4
	github.com/DataDog/datadog-agent/pkg/security/secl v0.34.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/log v0.34.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.34.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.34.0-rc.4
	github.com/DataDog/datadog-go v4.8.3+incompatible
	github.com/DataDog/datadog-operator v0.5.0-rc.2.0.20210402083916-25ba9a22e67a
	github.com/DataDog/ebpf-manager v0.0.0-20220125102432-266fda150ee4
	github.com/DataDog/gohai v0.0.0-20220112164844-3f118982b8ef
	github.com/DataDog/gopsutil v0.0.0-20211112180027-9aa392ae181a
	github.com/DataDog/nikos v1.7.1
	github.com/DataDog/sketches-go v1.2.1
	github.com/DataDog/viper v1.9.0
	github.com/DataDog/watermarkpodautoscaler v0.3.1-logs-attributes.2.0.20211014120627-6d6a5c559fc9
	github.com/DataDog/zstd v1.5.0
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f
	github.com/Masterminds/semver v1.5.0
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/Microsoft/go-winio v0.5.1
	github.com/Microsoft/hcsshim v0.9.1
	github.com/acobaugh/osrelease v0.1.0
	github.com/alecthomas/jsonschema v0.0.0-20210526225647-edb03dcab7bc
	github.com/alecthomas/participle v0.7.1
	github.com/alecthomas/repr v0.0.0-20181024024818-d37bc2a10ba1
	github.com/alecthomas/units v0.0.0-20210208195552-ff826a37aa15
	github.com/avast/retry-go v3.0.0+incompatible
	github.com/aws/aws-sdk-go v1.42.20
	github.com/beevik/ntp v0.3.0
	github.com/benbjohnson/clock v1.3.0
	github.com/benesch/cgosymbolizer v0.0.0-20190515212042-bec6fe6e597b
	github.com/bhmj/jsonslice v0.0.0-20200323023432-92c3edaad8e2
	github.com/blabber/go-freebsd-sysctl v0.0.0-20201130114544-503969f39d8f
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/cilium/ebpf v0.7.1-0.20211227144435-70d770f1e5f9
	github.com/clbanning/mxj v1.8.4
	github.com/cloudfoundry-community/go-cfclient v0.0.0-20210621174645-7773f7e22665
	github.com/containerd/cgroups v1.0.2
	github.com/containerd/containerd v1.5.9
	github.com/containerd/typeurl v1.0.2
	github.com/containernetworking/cni v0.8.1
	github.com/coreos/go-semver v0.3.0
	github.com/coreos/go-systemd v0.0.0-20190620071333-e64a0ec8b42a
	github.com/cri-o/ocicni v0.2.0
	github.com/cyphar/filepath-securejoin v0.2.3
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/docker v17.12.0-ce-rc1.0.20200916142827-bd33bbf0497b+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/dustin/go-humanize v1.0.0
	github.com/elastic/go-libaudit v0.4.0
	github.com/fatih/color v1.13.0
	github.com/florianl/go-conntrack v0.2.0
	github.com/freddierice/go-losetup v0.0.0-20170407175016-fc9adea44124
	github.com/go-ini/ini v1.66.3
	github.com/go-ole/go-ole v1.2.6
	github.com/go-openapi/spec v0.20.4
	github.com/gobwas/glob v0.2.3
	github.com/godbus/dbus v4.1.0+incompatible
	github.com/gogo/protobuf v1.3.2
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/google/gofuzz v1.2.0
	github.com/google/gopacket v1.1.19
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1
	github.com/gorilla/mux v1.8.0
	github.com/gosnmp/gosnmp v1.34.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/grpc-ecosystem/grpc-gateway v1.16.0
	github.com/h2non/filetype v1.1.2-0.20210602110014-3305bbb7ac7b
	github.com/hashicorp/consul/api v1.12.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru v0.5.4
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95
	github.com/iceber/iouring-go v0.0.0-20210726032807-b073cc83b2b8
	github.com/imdario/mergo v0.3.12
	github.com/iovisor/gobpf v0.2.0
	github.com/itchyny/gojq v0.12.6
	github.com/json-iterator/go v1.1.12
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/karrick/godirwalk v1.16.1
	github.com/kubernetes-sigs/custom-metrics-apiserver v0.0.0-20210311094424-0ca2b1909cdc
	github.com/lxn/walk v0.0.0-20191128110447-55ccb3a9f5c1
	github.com/lxn/win v0.0.0-20191128105842-2da648fda5b4
	github.com/mailru/easyjson v0.7.7
	github.com/mdlayher/netlink v1.4.1
	github.com/mholt/archiver/v3 v3.5.1
	github.com/miekg/dns v1.1.45
	github.com/mitchellh/mapstructure v1.4.3
	github.com/moby/sys/mountinfo v0.5.0
	github.com/mohae/deepcopy v0.0.0-20170603005431-491d3605edfb
	github.com/olekukonko/tablewriter v0.0.5
	github.com/open-policy-agent/opa v0.35.0
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.43.0
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417
	github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/richardartoul/molecule v0.0.0-20210914193524-25d8911bb85b
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da
	github.com/shirou/gopsutil v3.21.9+incompatible
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4
	github.com/skydive-project/go-debouncer v1.0.0
	github.com/spf13/afero v1.6.0
	github.com/spf13/cobra v1.3.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	github.com/theupdateframework/go-tuf v0.0.0-20210929155205-2707f22b6f31
	github.com/tinylib/msgp v1.1.6
	github.com/twmb/murmur3 v1.1.6
	github.com/urfave/negroni v1.0.0
	github.com/vishvananda/netlink v1.1.1-0.20210508154835-66ddd91f7ddd
	github.com/vishvananda/netns v0.0.0-20210104183010-2eb08e3e575f
	github.com/vmihailenco/msgpack/v4 v4.3.12
	github.com/xeipuuv/gojsonschema v1.2.0
	go.etcd.io/bbolt v1.3.6
	go.etcd.io/etcd/client/v2 v2.305.1
	go.opentelemetry.io/collector v0.43.1
	go.opentelemetry.io/collector/model v0.43.1
	go.uber.org/automaxprocs v1.4.0
	go.uber.org/multierr v1.7.0
	go.uber.org/zap v1.20.0
	go4.org/intern v0.0.0-20210108033219-3eb7198706b2
	golang.org/x/mobile v0.0.0-20201217150744-e6ae53a27f4f
	golang.org/x/net v0.0.0-20211111083644-e5c967477495
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211205182925-97ca703d548d
	golang.org/x/text v0.3.7
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	golang.org/x/tools v0.1.9
	gomodules.xyz/jsonpatch/v3 v3.0.1
	google.golang.org/genproto v0.0.0-20211208223120-3a66f561d7aa
	google.golang.org/grpc v1.43.0
	gopkg.in/DataDog/dd-trace-go.v1 v1.34.0
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

require (
	cloud.google.com/go v0.99.0 // indirect
	cloud.google.com/go/storage v1.12.0 // indirect
	code.cloudfoundry.org/cfhttp/v2 v2.0.0 // indirect
	code.cloudfoundry.org/clock v1.0.0 // indirect
	code.cloudfoundry.org/consuladapter v0.0.0-20200131002136-ac1daf48ba97 // indirect
	code.cloudfoundry.org/diego-logging-client v0.0.0-20200130234554-60ef08820a45 // indirect
	code.cloudfoundry.org/executor v0.0.0-20200218194701-024d0bdd52d4 // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20190809170250-f77fb823c7ee // indirect
	code.cloudfoundry.org/go-loggregator v7.4.0+incompatible // indirect
	code.cloudfoundry.org/gofileutils v0.0.0-20170111115228-4d0c80011a0f // indirect
	code.cloudfoundry.org/locket v0.0.0-20200131001124-67fd0a0fdf2d // indirect
	code.cloudfoundry.org/rep v0.0.0-20200325195957-1404b978e31e // indirect
	code.cloudfoundry.org/rfc5424 v0.0.0-20180905210152-236a6d29298a // indirect
	code.cloudfoundry.org/tlsconfig v0.0.0-20200131000646-bbe0f8da39b3 // indirect
	contrib.go.opencensus.io/exporter/prometheus v0.4.0 // indirect
	github.com/AlekSi/pointer v1.1.0 // indirect
	github.com/BurntSushi/toml v0.4.1 // indirect
	github.com/DataDog/extendeddaemonset v0.5.1-0.20210315105301-41547d4ff09c // indirect
	github.com/DataDog/gostackparse v0.5.0 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DisposaBoy/JsonConfigReader v0.0.0-20171218180944-5ea4d0ddac55 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/OneOfOne/xxhash v1.2.8 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/andybalholm/brotli v1.0.1 // indirect
	github.com/aptly-dev/aptly v1.4.1-0.20211102140819-ab2f5420c617 // indirect
	github.com/arduino/go-apt-client v0.0.0-20190812130613-5613f843fdc8 // indirect
	github.com/armon/go-metrics v0.3.10 // indirect
	github.com/awalterschulze/gographviz v2.0.1+incompatible // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/bmizerany/pat v0.0.0-20170815010413-6226ea591a40 // indirect
	github.com/cenkalti/backoff/v4 v4.1.2 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/containerd/continuity v0.1.0 // indirect
	github.com/containerd/fifo v1.0.0 // indirect
	github.com/containerd/ttrpc v1.1.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/dgryski/go-jump v0.0.0-20170409065014-e1f439676b57 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/emicklei/go-restful v2.14.3+incompatible // indirect
	github.com/emicklei/go-restful-swagger12 v0.0.0-20201014110547-68ccff494617 // indirect
	github.com/evanphx/json-patch v4.9.0+incompatible // indirect
	github.com/felixge/httpsnoop v1.0.2 // indirect
	github.com/florianl/go-tc v0.3.0 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-kit/log v0.1.0 // indirect
	github.com/go-logfmt/logfmt v0.5.0 // indirect
	github.com/go-logr/logr v1.2.1 // indirect
	github.com/go-logr/stdr v1.2.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.6 // indirect
	github.com/go-openapi/swag v0.19.15 // indirect
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/go-test/deep v1.0.5 // indirect
	github.com/godbus/dbus/v5 v5.0.4 // indirect
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/golang/glog v1.0.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/gax-go/v2 v2.1.1 // indirect
	github.com/googleapis/gnostic v0.5.1 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.0.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/serf v0.9.6 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/iancoleman/orderedmap v0.0.0-20190318233801-ac98e3ecb4b0 // indirect
	github.com/ianlancetaylor/cgosymbolizer v0.0.0-20201204192058-7acc97e53614 // indirect
	github.com/ianlancetaylor/demangle v0.0.0-20200824232613-28f6c0f3b639 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/itchyny/timefmt-go v0.1.3 // indirect
	github.com/jlaffaye/ftp v0.0.0-20200812143550-39e3779af0db // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/josharian/native v0.0.0-20200817173448-b6b71def0850 // indirect
	github.com/kjk/lzma v0.0.0-20161016003348-3fd93898850d // indirect
	github.com/klauspost/compress v1.14.1 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/knadh/koanf v1.4.0 // indirect
	github.com/lib/pq v1.10.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.5 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mdlayher/socket v0.0.0-20210307095302-262dc9984e00 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/mkrautz/goar v0.0.0-20150919110319-282caa8bd9da // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mostynb/go-grpc-compression v1.1.16 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/oliveagle/jsonpath v0.0.0-20180606110733-2e52cf6e6852 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.17.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opencontainers/runc v1.0.2 // indirect
	github.com/opencontainers/selinux v1.9.1 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pelletier/go-toml v1.9.4 // indirect
	github.com/philhofer/fwd v1.1.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.3 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/prometheus/statsd_exporter v0.21.0 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rs/cors v1.8.2 // indirect
	github.com/sassoftware/go-rpmutils v0.2.0 // indirect
	github.com/shirou/gopsutil/v3 v3.21.12 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/smira/go-ftp-protocol v0.0.0-20140829150050-066b75c2b70d // indirect
	github.com/smira/go-xz v0.0.0-20150414201226-0c531f070014 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20190923125748-758128399b1d // indirect
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00 // indirect
	github.com/tedsuo/rata v1.0.0 // indirect
	github.com/tent/canonical-json-go v0.0.0-20130607151641-96e4ba3a7613 // indirect
	github.com/tklauser/go-sysconf v0.3.9 // indirect
	github.com/tklauser/numcpus v0.3.0 // indirect
	github.com/ugorji/go/codec v1.1.7 // indirect
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/vito/go-sse v1.0.0 // indirect
	github.com/vmihailenco/tagparser v0.1.1 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/xor-gate/ar v0.0.0-20170530204233-5c72ae81e2b7 // indirect
	github.com/yashtewari/glob-intersection v0.0.0-20180916065949-5c77d914dd0b // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200910180754-dd1b699fc489 // indirect
	go.etcd.io/etcd/api/v3 v3.5.1 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.1 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.28.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.28.0 // indirect
	go.opentelemetry.io/contrib/zpages v0.28.0 // indirect
	go.opentelemetry.io/otel v1.3.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.26.0 // indirect
	go.opentelemetry.io/otel/internal/metric v0.26.0 // indirect
	go.opentelemetry.io/otel/metric v0.26.0 // indirect
	go.opentelemetry.io/otel/sdk v1.3.0 // indirect
	go.opentelemetry.io/otel/sdk/export/metric v0.26.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.26.0 // indirect
	go.opentelemetry.io/otel/trace v1.3.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20201222175341-b30ae309168e // indirect
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5 // indirect
	golang.org/x/mod v0.5.1 // indirect
	golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8 // indirect
	golang.org/x/term v0.0.0-20210220032956-6a3ed077a48d // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gomodules.xyz/orderedmap v0.1.0 // indirect
	google.golang.org/api v0.62.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	// See internal/patch/grpc-go-insecure/README.md for more details.
	google.golang.org/grpc/credentials/insecure v0.0.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/Knetic/govaluate.v3 v3.0.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	k8s.io/component-base v0.21.5 // indirect
	k8s.io/gengo v0.0.0-20201214224949-b6c5ce23f027 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.22 // indirect
	sigs.k8s.io/controller-runtime v0.7.2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.1.2 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
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

// k8s.io/component-base is incompatible with logr v1.x,
// remove once k8s.io/* is upgrade to v0.23.x or above (https://github.com/kubernetes/kubernetes/commit/cb6a6537)
replace (
	github.com/go-logr/logr => github.com/go-logr/logr v0.4.0
	github.com/go-logr/stdr => github.com/go-logr/stdr v0.4.0
)
