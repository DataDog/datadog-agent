module github.com/DataDog/datadog-agent/test/new-e2e

go 1.23.0

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
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def => ../../comp/otelcol/ddflareextension/def
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
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../pkg/version
	github.com/DataDog/datadog-agent/test/fakeintake => ../fakeintake
)

require (
	github.com/DataDog/agent-payload/v5 v5.0.138
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def v0.56.2
	github.com/DataDog/datadog-agent/pkg/util/optional v0.59.1
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.59.0
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.59.0
	github.com/DataDog/datadog-agent/pkg/util/testutil v0.56.2
	github.com/DataDog/datadog-agent/pkg/version v0.59.1
	github.com/DataDog/datadog-agent/test/fakeintake v0.56.0-rc.3
	github.com/DataDog/datadog-api-client-go v1.16.0
	github.com/DataDog/datadog-api-client-go/v2 v2.33.0
	// Are you bumping github.com/DataDog/test-infra-definitions ?
	// You should bump `TEST_INFRA_DEFINITIONS_BUILDIMAGES` in `.gitlab/common/test_infra_version.yml`
	// `TEST_INFRA_DEFINITIONS_BUILDIMAGES` matches the commit sha in the module version
	// Example: 	github.com/DataDog/test-infra-definitions v0.0.0-YYYYMMDDHHmmSS-0123456789AB
	// => TEST_INFRA_DEFINITIONS_BUILDIMAGES: 0123456789AB
	github.com/DataDog/test-infra-definitions v0.0.0-20241218140851-221bbc806266
	github.com/aws/aws-sdk-go-v2 v1.32.6
	github.com/aws/aws-sdk-go-v2/config v1.28.6
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.190.0
	github.com/aws/aws-sdk-go-v2/service/eks v1.51.0
	github.com/aws/aws-sdk-go-v2/service/ssm v1.55.2
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/docker/cli v27.4.0+incompatible
	github.com/docker/docker v27.4.0+incompatible
	github.com/fatih/color v1.18.0
	github.com/google/uuid v1.6.0
	github.com/kr/pretty v0.3.1
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c
	github.com/pkg/sftp v1.13.7
	github.com/pulumi/pulumi-aws/sdk/v6 v6.65.0
	github.com/pulumi/pulumi-awsx/sdk/v2 v2.19.0
	github.com/pulumi/pulumi-kubernetes/sdk/v4 v4.19.0
	github.com/pulumi/pulumi/sdk/v3 v3.142.0
	github.com/samber/lo v1.47.0
	github.com/stretchr/testify v1.10.0
	github.com/xeipuuv/gojsonschema v1.2.0
	golang.org/x/crypto v0.31.0
	golang.org/x/sys v0.28.0
	golang.org/x/term v0.27.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0
	k8s.io/api v0.31.4
	k8s.io/apimachinery v0.31.4
	k8s.io/cli-runtime v0.31.2
	k8s.io/client-go v0.31.3
	k8s.io/kubectl v0.31.2
)

require (
	dario.cat/mergo v1.0.1 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/DataDog/datadog-agent/comp/netflow/payload v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/proto v0.56.0-rc.3
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/zstd v1.5.6 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.1.3 // indirect
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/alessio/shellescape v1.4.2 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.7 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.47 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.36.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecs v1.52.2
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.4.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.24.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.28.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.2 // indirect
	github.com/aws/smithy-go v1.22.1 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/charmbracelet/bubbles v0.18.0 // indirect
	github.com/charmbracelet/bubbletea v0.25.0 // indirect
	github.com/charmbracelet/lipgloss v0.10.0 // indirect
	github.com/cloudflare/circl v1.3.8 // indirect
	github.com/containerd/console v1.0.4 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/cyphar/filepath-securejoin v0.3.4 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/distribution/reference v0.6.0 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.5.0 // indirect
	github.com/go-git/go-git/v5 v5.12.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/gnostic-models v0.6.9-0.20230804172637-c7be7c783f49 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl/v2 v2.22.0 // indirect
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
	github.com/moby/spdystream v0.4.0 // indirect
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
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/opentracing/basictracer-go v1.1.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pgavlin/fx v0.1.6 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/term v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/pulumi/appdash v0.0.0-20231130102222-75f619a67231 // indirect
	github.com/pulumi/esc v0.10.0 // indirect
	github.com/pulumi/pulumi-command/sdk v1.0.1 // indirect
	github.com/pulumi/pulumi-docker/sdk/v4 v4.5.7 // indirect
	github.com/pulumi/pulumi-libvirt/sdk v0.5.3 // indirect
	github.com/pulumi/pulumi-random/sdk/v4 v4.16.7 // indirect
	github.com/pulumi/pulumi-tls/sdk/v4 v4.11.1 // indirect
	github.com/pulumiverse/pulumi-time/sdk v0.1.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/skeema/knownhosts v1.2.2 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/texttheater/golang-levenshtein v1.0.1 // indirect
	github.com/tinylib/msgp v1.2.4 // indirect
	github.com/uber/jaeger-client-go v2.30.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	github.com/zclconf/go-cty v1.15.0 // indirect
	github.com/zorkian/go-datadog-api v2.30.0+incompatible // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.56.0 // indirect
	go.opentelemetry.io/otel v1.32.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.31.0 // indirect
	go.opentelemetry.io/otel/metric v1.32.0 // indirect
	go.opentelemetry.io/otel/trace v1.32.0 // indirect
	go.starlark.net v0.0.0-20231101134539-556fd59b42f6 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20241210194714-1829a127f884
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/net v0.33.0
	golang.org/x/oauth2 v0.23.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/text v0.21.0
	golang.org/x/time v0.8.0 // indirect
	golang.org/x/tools v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241104194629-dd2ea8efbc28 // indirect
	google.golang.org/grpc v1.67.1 // indirect
	google.golang.org/protobuf v1.35.2 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools/v3 v3.5.1 // indirect
	k8s.io/component-base v0.31.2 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240430033511-f0e62f92d13f // indirect
	k8s.io/utils v0.0.0-20240821151609-f90d01438635 // indirect
	lukechampine.com/frand v1.4.2 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/kustomize/api v0.17.2 // indirect
	sigs.k8s.io/kustomize/kyaml v0.17.1 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/trace v0.56.0-rc.3
	github.com/DataDog/datadog-go/v5 v5.6.0
	github.com/aws/aws-sdk-go v1.55.5
	github.com/aws/aws-sdk-go-v2/service/s3 v1.71.0
	github.com/aws/session-manager-plugin v0.0.0-20241119210807-82dc72922492
	github.com/digitalocean/go-libvirt v0.0.0-20240812180835-9c6c0a310c6c
	github.com/hairyhenderson/go-codeowners v0.7.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.59.0 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cheggaaa/pb v1.0.29 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/creack/pty v1.1.21 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.23.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/onsi/ginkgo/v2 v2.20.2 // indirect
	github.com/onsi/gomega v1.34.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/authorization/v2 v2.73.1 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/compute/v2 v2.73.1 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/containerservice/v2 v2.73.1 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/managedidentity/v2 v2.73.1 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/network/v2 v2.73.1 // indirect
	github.com/pulumi/pulumi-azure-native-sdk/v2 v2.73.1 // indirect
	github.com/pulumi/pulumi-eks/sdk/v3 v3.4.0 // indirect
	github.com/pulumi/pulumi-gcp/sdk/v7 v7.38.0 // indirect
	github.com/twinj/uuid v0.0.0-20151029044442-89173bcdda19 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.opentelemetry.io/collector/component v0.115.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.115.0 // indirect
	go.opentelemetry.io/collector/extension v0.115.0 // indirect
	go.opentelemetry.io/collector/pdata v1.21.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	google.golang.org/genproto v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20241104194629-dd2ea8efbc28 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
)
