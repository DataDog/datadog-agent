module github.com/DataDog/datadog-agent

go 1.15

// NOTE: Prefer using simple `require` directives instead of using `replace` if possible.
// See https://github.com/DataDog/datadog-agent/blob/main/docs/dev/gomodreplace.md
// for more details.

// Internal deps fix version
replace (
	github.com/cihub/seelog => github.com/cihub/seelog v0.0.0-20151216151435-d2c6e5aa9fbf // v2.6
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20180202092358-40e2722dffea
	github.com/docker/distribution => github.com/docker/distribution v2.7.1-0.20190104202606-0ac367fd6bee+incompatible
	github.com/florianl/go-conntrack => github.com/florianl/go-conntrack v0.2.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	github.com/iovisor/gobpf => github.com/DataDog/gobpf v0.0.0-20210322155958-9866ef4cd22c
	github.com/lxn/walk => github.com/lxn/walk v0.0.0-20180521183810-02935bac0ab8
	github.com/mholt/archiver => github.com/mholt/archiver v2.0.1-0.20171012052341-26cf5bb32d07+incompatible
	github.com/spf13/cast => github.com/DataDog/cast v1.3.1-0.20190301154711-1ee8c8bd14a3
	github.com/ugorji/go => github.com/ugorji/go v1.1.7
)

// pinned to grpc v1.27.0
replace (
	github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.12.2
	google.golang.org/grpc => github.com/grpc/grpc-go v1.27.0
)

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ./pkg/util/log
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
	github.com/DataDog/agent-payload v4.78.0+incompatible
	github.com/DataDog/datadog-agent/pkg/util/log v0.30.0-rc.7
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.30.0-rc.7
	github.com/DataDog/datadog-go v4.8.0+incompatible
	github.com/DataDog/datadog-operator v0.5.0-rc.2.0.20210402083916-25ba9a22e67a
	github.com/DataDog/ebpf v0.0.0-20210419131141-ea64821c9793
	github.com/DataDog/gohai v0.0.0-20210303102637-6b668acb50dd
	github.com/DataDog/gopsutil v0.0.0-20200624212600-1b53412ef321
	github.com/DataDog/mmh3 v0.0.0-20200316233529-f5b682d8c981 // indirect
	github.com/DataDog/nikos v1.0.0
	github.com/DataDog/sketches-go v1.0.0
	github.com/DataDog/viper v1.9.0
	github.com/DataDog/watermarkpodautoscaler v0.2.1-0.20210323121426-cfb2caa5613f
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/Microsoft/go-winio v0.4.17-0.20210211115548-6eac466e5fa3
	github.com/StackExchange/wmi v0.0.0-20210224194228-fe8f1750fd46 // indirect
	github.com/alecthomas/participle v0.7.1
	github.com/alecthomas/repr v0.0.0-20181024024818-d37bc2a10ba1
	github.com/andybalholm/brotli v1.0.1 // indirect
	github.com/avast/retry-go v2.7.0+incompatible
	github.com/aws/aws-sdk-go v1.35.24
	github.com/beevik/ntp v0.3.0
	github.com/benesch/cgosymbolizer v0.0.0-20190515212042-bec6fe6e597b
	github.com/bhmj/jsonslice v0.0.0-20200323023432-92c3edaad8e2
	github.com/blabber/go-freebsd-sysctl v0.0.0-20201130114544-503969f39d8f
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/clbanning/mxj v1.8.4
	github.com/cloudfoundry-community/go-cfclient v0.0.0-20201123235753-4f46d6348a05
	github.com/cobaugh/osrelease v0.0.0-20181218015638-a93a0a55a249
	github.com/containerd/cgroups v0.0.0-20210114181951-8a68de567b68
	github.com/containerd/containerd v1.5.0-beta.4
	github.com/containerd/typeurl v1.0.2
	github.com/coreos/go-semver v0.3.0
	github.com/coreos/go-systemd v0.0.0-20190620071333-e64a0ec8b42a
	github.com/davecgh/go-spew v1.1.1
	github.com/dgraph-io/ristretto v0.0.3
	github.com/docker/docker v17.12.0-ce-rc1.0.20200916142827-bd33bbf0497b+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/dustin/go-humanize v1.0.0
	github.com/elastic/go-libaudit v0.4.0
	github.com/fatih/color v1.12.0
	github.com/fatih/structtag v1.2.0
	github.com/florianl/go-conntrack v0.1.1-0.20191002182014-06743d3a59db
	github.com/freddierice/go-losetup v0.0.0-20170407175016-fc9adea44124
	github.com/go-ini/ini v1.62.0
	github.com/go-ole/go-ole v1.2.5
	github.com/go-openapi/spec v0.20.3
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/go-test/deep v1.0.5 // indirect
	github.com/gobwas/glob v0.2.3
	github.com/godbus/dbus v4.1.0+incompatible
	github.com/gogo/protobuf v1.3.2
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.3 // indirect
	github.com/google/gofuzz v1.2.0
	github.com/google/gopacket v1.1.19
	github.com/google/pprof v0.0.0-20210125172800-10e9aeb4a998
	github.com/gorilla/mux v1.8.0
	github.com/gosnmp/gosnmp v1.31.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/grpc-ecosystem/grpc-gateway v1.16.0
	github.com/h2non/filetype v1.1.1
	github.com/hashicorp/consul/api v1.8.1
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru v0.5.4
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/ianlancetaylor/cgosymbolizer v0.0.0-20201204192058-7acc97e53614 // indirect
	github.com/iceber/iouring-go v0.0.0-20210723144509-ddbccb61dc5b
	github.com/iovisor/gobpf v0.0.0
	github.com/itchyny/gojq v0.12.4
	github.com/json-iterator/go v1.1.11
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/klauspost/compress v1.11.12 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/kubernetes-sigs/custom-metrics-apiserver v0.0.0-20210311094424-0ca2b1909cdc
	github.com/lib/pq v1.10.0 // indirect
	github.com/lxn/walk v0.0.0-20191128110447-55ccb3a9f5c1
	github.com/lxn/win v0.0.0-20191128105842-2da648fda5b4
	github.com/mailru/easyjson v0.7.7
	github.com/mdlayher/netlink v1.4.1
	github.com/mholt/archiver/v3 v3.5.0
	github.com/miekg/dns v1.1.43
	github.com/mitchellh/copystructure v1.1.2 // indirect
	github.com/mitchellh/mapstructure v1.4.1
	github.com/moby/sys/mountinfo v0.4.1
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/olekukonko/tablewriter v0.0.5
	github.com/oliveagle/jsonpath v0.0.0-20180606110733-2e52cf6e6852 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20200929063507-e6143ca7d51d
	github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pierrec/lz4/v4 v4.1.3 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.10.0
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da
	github.com/shirou/gopsutil v3.21.5+incompatible
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4
	github.com/shuLhan/go-bindata v3.6.1+incompatible
	github.com/spf13/afero v1.6.0
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00 // indirect
	github.com/tinylib/msgp v1.1.6
	github.com/tklauser/go-sysconf v0.3.4 // indirect
	github.com/twmb/murmur3 v1.1.5
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/urfave/negroni v1.0.0
	github.com/vishvananda/netlink v1.1.1-0.20210508154835-66ddd91f7ddd
	github.com/vishvananda/netns v0.0.0-20210104183010-2eb08e3e575f
	github.com/vito/go-sse v1.0.0 // indirect
	github.com/vmihailenco/msgpack/v4 v4.3.11
	github.com/xeipuuv/gojsonschema v0.0.0-20180618132009-1d523034197f
	go.etcd.io/etcd/client/v2 v2.305.0
	go.opencensus.io v0.22.6 // indirect
	go.opentelemetry.io/otel v0.20.0
	go.uber.org/automaxprocs v1.4.0
	golang.org/x/crypto v0.0.0-20210317152858-513c2a44f670 // indirect
	golang.org/x/mobile v0.0.0-20201217150744-e6ae53a27f4f
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210601080250-7ecdf8ef093b
	golang.org/x/text v0.3.6
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	golang.org/x/tools v0.1.4
	gomodules.xyz/jsonpatch/v3 v3.0.1
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c
	google.golang.org/grpc v1.38.0
	gopkg.in/DataDog/dd-trace-go.v1 v1.31.1
	gopkg.in/Knetic/govaluate.v3 v3.0.0 // indirect
	gopkg.in/ini.v1 v1.55.0 // indirect
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/apiserver v0.20.5
	k8s.io/autoscaler/vertical-pod-autoscaler v0.9.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/cri-api v0.20.1
	k8s.io/klog v1.0.1-0.20200310124935-4ad0115ba9e4 // Min version that includes fix for Windows Nano
	k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
	k8s.io/kube-state-metrics/v2 v2.0.1-0.20210419063836-dfab196f138a
	k8s.io/kubernetes v1.20.5
	k8s.io/metrics v0.20.5
)

// Pinned to kubernetes-v0.20.5
replace (
	k8s.io/api => k8s.io/api v0.20.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.5
	k8s.io/apiserver => k8s.io/apiserver v0.20.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.5
	k8s.io/client-go => k8s.io/client-go v0.20.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.5
	k8s.io/code-generator => k8s.io/code-generator v0.20.5
	k8s.io/component-base => k8s.io/component-base v0.20.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.20.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.20.5
	k8s.io/cri-api => k8s.io/cri-api v0.20.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.5
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.5
	k8s.io/kubectl => k8s.io/kubectl v0.20.5
	k8s.io/kubelet => k8s.io/kubelet v0.20.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.5
	k8s.io/metrics => k8s.io/metrics v0.20.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.20.3-rc.0
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.5
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.20.5
	k8s.io/sample-controller => k8s.io/sample-controller v0.20.5
)

replace gopkg.in/DataDog/dd-trace-go.v1 => gopkg.in/DataDog/dd-trace-go.v1 v1.30.0

// Remove once the PR kubernetes/kube-state-metrics#1516 is merged and released.
replace k8s.io/kube-state-metrics/v2 => github.com/ahmed-mez/kube-state-metrics/v2 v2.1.0-rc.0.0.20210629115837-e46f17606d22

replace github.com/aptly-dev/aptly => github.com/lebauce/aptly v0.7.2-0.20201005164315-09522984a976
