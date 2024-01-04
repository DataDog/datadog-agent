// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	infraK8s "github.com/DataDog/test-infra-definitions/components/kubernetes"
	commonvm "github.com/DataDog/test-infra-definitions/components/vm"
	"github.com/DataDog/test-infra-definitions/scenarios/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
	pulumiK8s "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
)

type kindVMTestSuite struct {
	suite.Suite

	infraVM    commonvm.VM
	VM         *client.PulumiStackVM
	Fakeintake *fakeintake.Client
}

func TestKindVMTestSuite(t *testing.T) {
	suite.Run(t, &kindVMTestSuite{})
}

func (s *kindVMTestSuite) SetupSuite() {
	ctx := context.Background()

	deployFunc := deployKindVM(s)
	_, stackOutput, err := infra.GetStackManager().
		GetStack(ctx, "kind-cluster", nil, deployFunc, false)
	s.Require().NoError(err)

	var clientData *commonvm.ClientData
	clientData, err = s.infraVM.Deserialize(stackOutput)
	s.Require().NoError(err)

	s.VM = client.NewPulumiStackVM(s.infraVM)
	s.VM.VM, err = client.NewVMClient(s.T(), &clientData.Connection, s.infraVM.GetOS().GetType())
	s.Require().NoError(err)

	s.Fakeintake = fakeintake.NewClient(
		fmt.Sprintf("http://%s", stackOutput.Outputs["fakeintake-host"].Value.(string)))

	s.installKubectl()
}

// deployKindVM generates a func used by Pulumi to deploy the environment.
// It also updates the test suite with a reference to the new VM.
func deployKindVM(s *kindVMTestSuite) func(*pulumi.Context) error {
	return func(ctx *pulumi.Context) error {
		vm, err := ec2vm.NewUnixEc2VM(ctx)
		if err != nil {
			return err
		}
		awsEnv := vm.Infra.GetAwsEnvironment()
		s.infraVM = vm

		kubeConfigCommand, kubeConfig, err := infraK8s.NewKindCluster(
			vm.UnixVM, awsEnv.CommonNamer.ResourceName("kind"), "amd64", awsEnv.KubernetesVersion())
		if err != nil {
			return err
		}

		ctx.Export("kubeconfig", kubeConfig)

		kindKubeProvider, err := pulumiK8s.NewProvider(
			ctx, awsEnv.Namer.ResourceName("k8s-provider"),
			&pulumiK8s.ProviderArgs{
				EnableServerSideApply: pulumi.BoolPtr(true),
				Kubeconfig:            kubeConfig,
			},
			utils.PulumiDependsOn(kubeConfigCommand))
		if err != nil {
			return err
		}

		fakeIntake, err := aws.NewEcsFakeintake(awsEnv)
		if err != nil {
			return err
		}

		clusterName := ctx.Stack()
		customValues := fmt.Sprintf(`
datadog:
  kubelet:
    tlsVerify: false
  clusterName: "%s"
agents:
  useHostNetwork: true
`, clusterName)

		helmComponent, err := agent.NewHelmInstallation(
			*awsEnv.CommonEnvironment,
			agent.HelmInstallationArgs{
				KubeProvider: kindKubeProvider,
				Namespace:    "datadog",
				ValuesYAML:   pulumi.AssetOrArchiveArray{pulumi.NewStringAsset(customValues)},
				Fakeintake:   fakeIntake,
			},
			nil,
		)
		if err != nil {
			return err
		}

		ctx.Export("kube-cluster-name", pulumi.String(clusterName))
		ctx.Export("agent-linux-helm-install-name", helmComponent.LinuxHelmReleaseName)
		ctx.Export("agent-linux-helm-install-status", helmComponent.LinuxHelmReleaseStatus)

		_, err = cpustress.K8sAppDefinition(
			*awsEnv.CommonEnvironment, kindKubeProvider, "workload-cpustress")
		if err != nil {
			return err
		}

		return nil
	}
}

// installKubectl installs the kubectl tool on the VM
func (s *kindVMTestSuite) installKubectl() {
	// Source: https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/
	s.VM.Execute(`curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"`)
	s.VM.Execute("sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl")
	s.VM.Execute("kubectl config set-context --current --namespace datadog")
}

// execAgentPod executes the given command against the datadog agent pod
func (s *kindVMTestSuite) execAgentPod(container string, command string) string {
	podName := strings.TrimSpace(s.VM.Execute("kubectl get pod -o name -l app=dda-datadog"))
	return s.VM.Execute(fmt.Sprintf("kubectl exec %s -c %s -- %s", podName, container, command))
}

// getAgentStatus runs the status command and returns the result as a JSON string.
func (s *kindVMTestSuite) getAgentStatus() string {
	return cleanJSONOutput(s.execAgentPod("agent", "agent status --json"))
}

func (s *kindVMTestSuite) TestProcessCheck() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		status := s.getAgentStatus()
		assertRunningChecks(c, status, []string{"process", "rtprocess"}, false)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	s.EventuallyWithT(func(c *assert.CollectT) {
		var err error
		payloads, err = s.Fakeintake.GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(s.T(), payloads, false, stressK8sProcName)
}

func (s *kindVMTestSuite) TestManualProcessCheck() {
	check := cleanJSONOutput(s.execAgentPod("process-agent", "process-agent check process --json"))
	assertManualProcessCheck(s.T(), check, false, stressK8sProcName)
}

func (s *kindVMTestSuite) TestManualContainerCheck() {
	check := cleanJSONOutput(
		s.execAgentPod("process-agent", "process-agent check container --json"))
	assertManualContainerCheck(s.T(), check, stressK8sImageTag)
}

func (s *kindVMTestSuite) TestManualProcessDiscoveryCheck() {
	check := cleanJSONOutput(
		s.execAgentPod("process-agent", "process-agent check process_discovery --json"))
	assertManualProcessDiscoveryCheck(s.T(), check, stressK8sProcName)
}
