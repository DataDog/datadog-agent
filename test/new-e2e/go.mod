module github.com/DataDog/datadog-agent/test/new-e2e

go 1.18

// Do not upgrade Pulumi plugins to versions different from `test-infra-definitions`.
// The plugin versions NEED to be aligned.
// TODO: Implement hard check in CI

replace github.com/DataDog/datadog-agent/test/fakeintake => ../fakeintake

require (
	github.com/DataDog/datadog-agent/test/fakeintake v0.46.0-rc.2
	// Are you bumping github.com/DataDog/test-infra-definitions ?
	// You should bump the image version at the `image` field in `.new_e2e_template` job
	// in .gitlab/e2e.yml to 486234852809.dkr.ecr.us-east-1.amazonaws.com/ci/test-infra-definitions/runner:<commit_sha>
	// Commit sha matches the first 8 digits of the commit sha in the module version
	// Example: 	github.com/DataDog/test-infra-definitions v0.0.0-YYYYMMDDHHmmSS-0123456789AB
	// => image: 486234852809.dkr.ecr.us-east-1.amazonaws.com/ci/test-infra-definitions/runner:01234567
	github.com/DataDog/test-infra-definitions v0.0.0-20230619131020-fb813362e3e0
	github.com/aws/aws-sdk-go-v2 v1.18.1
	github.com/aws/aws-sdk-go-v2/config v1.18.27
	github.com/aws/aws-sdk-go-v2/service/ssm v1.36.4
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cenkalti/backoff/v4 v4.2.1
	github.com/fatih/color v1.13.0
	github.com/pulumi/pulumi-command/sdk v0.7.2
	github.com/pulumi/pulumi/sdk/v3 v3.72.2
	github.com/sethvargo/go-retry v0.2.4
	github.com/stretchr/testify v1.8.3
	golang.org/x/crypto v0.10.0
	golang.org/x/sys v0.9.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0
)

require (
	github.com/DataDog/agent-payload/v5 v5.0.73 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20230528122434-6f98819771a1 // indirect
	github.com/acomagu/bufpipe v1.0.4 // indirect
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/alessio/shellescape v1.4.1 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.13.26 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.28 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.35 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecs v1.27.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.28 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.12.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.14.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.19.2 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/cheggaaa/pb v1.0.29 // indirect
	github.com/cloudflare/circl v1.3.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/djherbis/times v1.5.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.4.1 // indirect
	github.com/go-git/go-git/v5 v5.7.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.1.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl/v2 v2.17.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/opentracing/basictracer-go v1.1.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/term v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/pulumi/pulumi-aws/sdk/v5 v5.41.0 // indirect
	github.com/pulumi/pulumi-awsx/sdk v1.0.2 // indirect
	github.com/pulumi/pulumi-docker/sdk/v3 v3.6.1 // indirect
	github.com/pulumi/pulumi-kubernetes/sdk/v3 v3.29.1 // indirect
	github.com/pulumi/pulumi-libvirt/sdk v0.4.0 // indirect
	github.com/pulumi/pulumi-random/sdk/v4 v4.13.2 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.0 // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/skeema/knownhosts v1.1.1 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/texttheater/golang-levenshtein v1.0.1 // indirect
	github.com/tweekmonster/luser v0.0.0-20161003172636-3fa38070dbd7 // indirect
	github.com/uber/jaeger-client-go v2.30.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/zclconf/go-cty v1.13.2 // indirect
	github.com/zorkian/go-datadog-api v2.30.0+incompatible // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20230522175609-2e198f4a06a1 // indirect
	golang.org/x/mod v0.11.0 // indirect
	golang.org/x/net v0.11.0 // indirect
	golang.org/x/term v0.9.0 // indirect
	golang.org/x/text v0.10.0 // indirect
	golang.org/x/tools v0.10.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/grpc v1.56.0 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	lukechampine.com/frand v1.4.2 // indirect
	sourcegraph.com/sourcegraph/appdash v0.0.0-20211028080628-e2786a622600 // indirect
)
