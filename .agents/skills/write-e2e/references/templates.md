# Suite skeletons

Load when you need a skeleton other than the Linux-host one in the skill body, or when splitting a suite across operating systems.

Each skeleton omits the license header for brevity — copy the four-line header from any file in `test/new-e2e/tests/`. Every entry point calls `t.Parallel()` first. Runnable versions of most of these live in `test/new-e2e/examples/`.

## Docker host

`environments.DockerHost`. The agent runs in a container on an EC2 host; reach it through `Env().Agent.Client` and the host through `Env().RemoteHost`.

```go
import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	scenariodocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
)

type dockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestDocker(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &dockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(
			scenariodocker.WithAgentOptions(
				// Reaches the agent process; WithEnvironmentVariables would not.
				dockeragentparams.WithAgentServiceEnvVariable("DD_LOG_LEVEL", pulumi.String("debug")),
			),
		),
	)))
}
```

Add a workload container by nesting the compose option inside the agent options — it belongs to `dockeragentparams`, not to the scenario package:

```go
scenariodocker.WithAgentOptions(
	dockeragentparams.WithExtraComposeManifest("workload", pulumi.String(composeYAML)),
)
```

Pull every image in that manifest through the ECR cache. `test/new-e2e/tests/agent-log-pipelines/listener/listener_test.go` is a working example.

## Kubernetes (kind on EC2)

`environments.Kubernetes`. The default choice for Kubernetes behavior; swap `kindvm` for `eks` only when the behavior is EKS-specific.

```go
import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

//go:embed fixtures/helm_values.yaml
var helmValues string

type myKindSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestMyKindSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &myKindSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenariokindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(helmValues),
				),
			),
		),
	))
}
```

`WithHelmValues` is repeatable and each call layers another document, so a shared base can be combined with a per-suite override. Cluster access:

```go
s.Env().KubernetesCluster.Client()                                              // kubernetes.Interface
s.Env().KubernetesCluster.KubernetesClient.PodExec(ns, pod, container, []string{"ls"})
```

Deploy a workload with `scenariokindvm.WithWorkloadApp(func(e config.Env, p *kubernetes.Provider) (*compkube.Workload, error) { ... })`; ready-made apps live in `test/e2e-framework/components/datadog/apps/`.

## ECS

```go
import (
	"testing"

	scenecs "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ecs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/ecs"
)

type myECSSuite struct {
	e2e.BaseSuite[environments.ECS]
}

func TestMyECSSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &myECSSuite{}, e2e.WithProvisioner(
		ecs.Provisioner(ecs.WithRunOptions(scenecs.WithECSOptions(scenecs.WithLinuxNodeGroup()))),
	))
}
```

Tasks are reached through `s.Env().ECSCluster.ECSClient` (`ListTasks`, `ExecCommand(taskARN, container, cmd)`).

## Windows host

Reach for this only when the test needs a Windows-specific scenario component — Active Directory, Defender, FIPS mode, test signing — or is an MSI installer test. Ordinary Windows behavior runs on plain `environments.Host` with `ec2.WithOS(e2eos.WindowsServerDefault)`; use the cross-OS split below, or extend a suite that already does this, rather than provisioning a second one.

Within Windows, use `winawshost`: the Windows CI templates depend on AWS-side MSI artifacts. A Windows provisioner also exists for Azure (`winazurehost`, flat options) and reportedly boots faster, but nothing in `tests/` uses it and no CI job wires it, so treat it as unproven rather than the default.

MSI installer tests belong in `test/new-e2e/tests/windows/install-test/` on top of `BaseAgentInstallerSuite` rather than this skeleton. Read `test/new-e2e/tests/windows/AGENTS.md` before writing either.

```go
import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenwin "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
)

type winSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

func TestWindows(t *testing.T) {
	t.Parallel()
	// AWS provisioners nest their options inside WithRunOptions.
	e2e.Run(t, &winSuite{}, e2e.WithProvisioner(winawshost.Provisioner(
		winawshost.WithRunOptions(
			scenwin.WithAgentOptions(agentparams.WithAgentConfig("log_level: debug")),
		),
	)))
}
```

Before writing PowerShell inline, check `test/new-e2e/tests/windows/common/` — services, registry, ACLs, event logs, crash dumps, local users, and filesystem snapshots already have Go wrappers.

## Splitting one suite across operating systems

Put the assertions in a shared body and vary only the OS descriptor, so the two platforms cannot drift. Each entry point needs its own named suite type embedding the shared one — `test/new-e2e/AGENTS.md` § "Each entry point needs its own suite type" has the reason.

`myfeature_common_test.go`:

```go
type myFeatureSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor e2eos.Descriptor
}

func (s *myFeatureSuite) suiteOptions() []e2e.SuiteOption {
	return []e2e.SuiteOption{e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(s.descriptor))),
	))}
}

func (s *myFeatureSuite) confDir() string {
	if s.descriptor.Family() == e2eos.WindowsFamily {
		return "C:/ProgramData/Datadog/conf.d"
	}
	return "/etc/datadog-agent/conf.d"
}
```

`myfeature_nix_test.go`:

```go
// A distinct type per entry point keeps the two suites on separate stacks.
type myFeatureLinuxSuite struct {
	myFeatureSuite
}

func TestMyFeatureLinux(t *testing.T) {
	t.Parallel()
	s := &myFeatureLinuxSuite{myFeatureSuite{descriptor: e2eos.Ubuntu2204}}
	e2e.Run(t, s, s.suiteOptions()...)
}
```

`myfeature_win_test.go` mirrors it with `myFeatureWindowsSuite` and `e2eos.WindowsServerDefault`. `test/new-e2e/tests/agent-runtimes/infra_basic_*_test.go` is the in-tree version of this pattern.

CI selects between them by name (`EXTRA_PARAMS: --skip "Windows"` on Linux jobs), so name the Windows entry point so that filter matches.

## Fixtures

```go
import _ "embed"

//go:embed fixtures/myfeature.yaml
var myFeatureConfig string
```

Keep fixtures in a `fixtures/` directory beside the test. A YAML snippet of ten lines or fewer is clearer inline as a `const`.

## Marking a known flake

`flake.Mark(t)` unconditionally, `flake.MarkOnLog(t, text)` or `flake.MarkOnLogRegex(t, pattern)` to mark only when a known symptom appears, and `flake.MarkOnJobName(t, jobNames...)` to scope it to specific CI jobs.

```go
import "github.com/DataDog/datadog-agent/pkg/util/testutil/flake"

func (s *mySuite) TestSometimesFlaky() {
	flake.MarkOnLog(s.T(), "connection reset by peer")
	// …
}
```

Marking is for a flake you have diagnosed and cannot yet fix. A newly written test that flakes is a defect in the test — the reliability rules in `test/new-e2e/codereview_guideline.md` cover the usual causes.

## Collecting diagnostics on failure

```go
func (s *mySuite) AfterTest(suiteName, testName string) {
	s.BaseSuite.AfterTest(suiteName, testName)
	if !s.T().Failed() {
		return
	}
	logs, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager -n 500")
	if err == nil {
		// SessionOutputDir is per suite, so write into the per-test subdirectory
		// that BaseSuite.AfterTest already created — a fixed filename at the top
		// level gets truncated by the next failing test.
		dir := filepath.Join(s.SessionOutputDir(), common.SanitizeDirectoryName(testName))
		_ = os.WriteFile(filepath.Join(dir, "agent-journal.log"), []byte(logs), 0o644)
	}
}
```

`common` here is `test/e2e-framework/testing/utils/common`. Write to the session output directory rather than the job log; the infrastructure is gone by the time anyone reads the failure, and large inline dumps make GitLab logs unusable.
