module github.com/DataDog/datadog-agent/test/new-e2e

go 1.22.5

toolchain go1.22.8

// Do not upgrade Pulumi plugins to versions different from `test-infra-definitions`.
// The plugin versions NEED to be aligned.
// TODO: Implement hard check in CI

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../comp/api/api/def
	github.com/DataDog/datadog-agent/comp/api/def => ../../comp/api/def
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../comp/core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../comp/core/flare/types
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../comp/core/secrets
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ../../comp/core/tagger/types
	github.com/DataDog/datadog-agent/comp/core/tagger/utils => ../../comp/core/tagger/utils
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../comp/core/telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../comp/def
	github.com/DataDog/datadog-agent/comp/netflow/payload => ../../comp/netflow/payload
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/model => ../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/proto => ../../pkg/proto
	github.com/DataDog/datadog-agent/pkg/trace => ../../pkg/trace
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system => ../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/tagger => ../../pkg/util/tagger
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../pkg/version
	github.com/DataDog/datadog-agent/test/fakeintake => ../fakeintake
)

require (
	github.com/DataDog/agent-payload/v5 v5.0.134
	github.com/DataDog/datadog-agent/pkg/util/optional v0.56.2
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.56.2
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.2
	github.com/DataDog/datadog-agent/pkg/util/testutil v0.56.2
	github.com/DataDog/datadog-agent/pkg/version v0.56.0-rc.3
	github.com/DataDog/datadog-agent/test/fakeintake v0.56.0-rc.3
	github.com/DataDog/datadog-api-client-go v1.16.0
	github.com/DataDog/datadog-api-client-go/v2 v2.27.0
	// Are you bumping github.com/DataDog/test-infra-definitions ?
	// You should bump `TEST_INFRA_DEFINITIONS_BUILDIMAGES` in `.gitlab/common/test_infra_version.yml`
	// `TEST_INFRA_DEFINITIONS_BUILDIMAGES` matches the commit sha in the module version
	// Example: 	github.com/DataDog/test-infra-definitions v0.0.0-YYYYMMDDHHmmSS-0123456789AB
	// => TEST_INFRA_DEFINITIONS_BUILDIMAGES: 0123456789AB
	github.com/DataDog/test-infra-definitions v0.0.0-20241010155348-7e55b9e3279a
	github.com/aws/aws-sdk-go-v2 v1.32.0
	github.com/aws/aws-sdk-go-v2/config v1.27.40
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.164.2
	github.com/aws/aws-sdk-go-v2/service/eks v1.44.1
	github.com/aws/aws-sdk-go-v2/service/ssm v1.50.7
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/docker/cli v24.0.7+incompatible
	github.com/docker/docker v25.0.6+incompatible
	github.com/fatih/color v1.16.0
	github.com/google/uuid v1.6.0
	github.com/kr/pretty v0.3.1
	github.com/mitchellh/mapstructure v1.5.0
	github.com/pkg/sftp v1.13.6
	github.com/pulumi/pulumi-aws/sdk/v6 v6.54.2
	github.com/pulumi/pulumi-awsx/sdk/v2 v2.14.0
	github.com/pulumi/pulumi-eks/sdk/v2 v2.7.8
	github.com/pulumi/pulumi-kubernetes/sdk/v4 v4.17.1
	github.com/pulumi/pulumi/sdk/v3 v3.133.0
	github.com/samber/lo v1.47.0
	github.com/stretchr/testify v1.9.0
	github.com/xeipuuv/gojsonschema v1.2.0
	golang.org/x/crypto v0.28.0
	golang.org/x/sys v0.26.0
	golang.org/x/term v0.25.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0
	k8s.io/api v0.30.2
	k8s.io/apimachinery v0.30.2
	k8s.io/cli-runtime v0.30.2
	k8s.io/client-go v0.30.2
	k8s.io/kubectl v0.30.2
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/DataDog/datadog-agent/comp/netflow/payload v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/proto v0.56.0-rc.3
	github.com/DataDog/mmh3 v0.0.0-20200805151601-30884ca2197a // indirect
	github.com/DataDog/zstd v1.5.5 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/ProtonMail/go-crypto v1.0.0 // indirect
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/alessio/shellescape v1.4.2 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.6 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.38 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.14 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.19 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.19 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.32.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecs v1.45.2
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.4.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.65.0
	github.com/aws/aws-sdk-go-v2/service/sso v1.23.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.27.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.31.4 // indirect
	github.com/aws/smithy-go v1.22.0 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/charmbracelet/bubbles v0.18.0 // indirect
	github.com/charmbracelet/bubbletea v0.25.0 // indirect
	github.com/charmbracelet/lipgloss v0.10.0 // indirect
	github.com/cheggaaa/pb v1.0.29 // indirect
	github.com/cloudflare/circl v1.3.7 // indirect
	github.com/containerd/console v1.0.4 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.4 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/distribution/reference v0.5.0 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/fvbommel/sortorder v1.1.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.5.0 // indirect
	github.com/go-git/go-git/v5 v5.12.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20221118152302-e6195bd50e26 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl/v2 v2.20.1 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/muesli/termenv v0.15.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opentracing/basictracer-go v1.1.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pgavlin/fx v0.1.6 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/term v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/pulumi/appdash v0.0.0-20231130102222-75f619a67231 // indirect
	github.com/pulumi/esc v0.9.1 // indirect
	github.com/pulumi/pulumi-command/sdk v1.0.1 // indirect
	github.com/pulumi/pulumi-docker/sdk/v4 v4.5.5 // indirect
	github.com/pulumi/pulumi-libvirt/sdk v0.4.7 // indirect
	github.com/pulumi/pulumi-random/sdk/v4 v4.16.6 // indirect
	github.com/pulumi/pulumi-tls/sdk/v4 v4.11.1 // indirect
	github.com/pulumiverse/pulumi-time/sdk v0.1.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/skeema/knownhosts v1.2.2 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/cobra v1.8.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/texttheater/golang-levenshtein v1.0.1 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	github.com/uber/jaeger-client-go v2.30.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	github.com/zclconf/go-cty v1.14.4 // indirect
	github.com/zorkian/go-datadog-api v2.30.0+incompatible
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.44.0 // indirect
	go.opentelemetry.io/otel v1.30.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.28.0 // indirect
	go.opentelemetry.io/otel/metric v1.30.0 // indirect
	go.opentelemetry.io/otel/sdk v1.28.0 // indirect
	go.opentelemetry.io/otel/trace v1.30.0 // indirect
	go.starlark.net v0.0.0-20230525235612-a134d8f9ddca // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20241004190924-225e2abe05e6
	golang.org/x/mod v0.21.0 // indirect
	golang.org/x/net v0.30.0 // indirect
	golang.org/x/oauth2 v0.18.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/text v0.19.0
	golang.org/x/time v0.7.0 // indirect
	golang.org/x/tools v0.26.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240701130421-f6361c86f094 // indirect
	google.golang.org/grpc v1.64.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools/v3 v3.5.0 // indirect
	k8s.io/component-base v0.30.2 // indirect
	k8s.io/klog/v2 v2.120.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
	lukechampine.com/frand v1.4.2 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/kustomize/api v0.13.5-0.20230601165947-6ce0bf390ce3 // indirect
	sigs.k8s.io/kustomize/kyaml v0.14.3-0.20230601165947-6ce0bf390ce3 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/trace v0.56.0-rc.3
	github.com/DataDog/datadog-go/v5 v5.5.0
	github.com/digitalocean/go-libvirt v0.0.0-20240812180835-9c6c0a310c6c
	github.com/hairyhenderson/go-codeowners v0.5.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/secrets v0.56.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/tagger v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.56.2 // indirect
	github.com/DataDog/viper v1.13.5 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/pulumi/pulumi-azure-native-sdk/authorization/v2 v2.60.0 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/compute/v2 v2.56.0 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/containerservice/v2 v2.59.0 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/network/v2 v2.59.0 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/v2 v2.60.0 // indirect
	github.com/pulumi/pulumi-gcp/sdk/v6 v6.67.1 // indirect
	github.com/pulumi/pulumi-gcp/sdk/v7 v7.38.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.4 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
)
