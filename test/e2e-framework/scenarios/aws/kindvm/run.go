package kindvm

import (
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/helm"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/dogstatsd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/etcd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/mutatedbyadmissioncontroller"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/prometheus"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/redis"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/tracegen"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operator"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"

	localKubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/components/kubernetes/vpa"
	"github.com/DataDog/test-infra-definitions/components/os"
	resAws "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context) error {
	awsEnv, err := resAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	osDesc := os.DescriptorFromString(awsEnv.InfraOSDescriptor(), os.AmazonLinuxECSDefault)
	vm, err := ec2.NewVM(awsEnv, "kind", ec2.WithOS(osDesc))
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	kindClusterName := ctx.Stack()

	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, vm)
	if err != nil {
		return err
	}

	kindCluster, err := localKubernetes.NewKindCluster(&awsEnv, vm, "kind", awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
	if err != nil {
		return err
	}
	if err := kindCluster.Export(ctx, nil); err != nil {
		return err
	}

	// Building Kubernetes provider
	kindKubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		Kubeconfig:            kindCluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
		DeleteUnreachable:     pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}

	vpaCrd, err := vpa.DeployCRD(&awsEnv, kindKubeProvider)
	if err != nil {
		return err
	}
	dependsOnVPA := utils.PulumiDependsOn(vpaCrd)

	var fakeIntake *fakeintakeComp.Fakeintake
	if awsEnv.AgentUseFakeintake() {
		fakeIntakeOptions := []fakeintake.Option{
			fakeintake.WithMemory(2048),
		}
		if awsEnv.InfraShouldDeployFakeintakeWithLB() {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
		}

		if awsEnv.AgentUseDualShipping() {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithoutDDDevForwarding())
		}

		if storeType := awsEnv.AgentFakeintakeStoreType(); storeType != "" {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithStoreType(storeType))
		}

		if retentionPeriod := awsEnv.AgentFakeintakeRetentionPeriod(); retentionPeriod != "" {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithRetentionPeriod(retentionPeriod))
		}

		if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, kindCluster.Name(), fakeIntakeOptions...); err != nil {
			return err
		}

		if err := fakeIntake.Export(awsEnv.Ctx(), nil); err != nil {
			return err
		}
	}

	var dependsOnDDAgent pulumi.ResourceOption

	// Deploy the agent
	if awsEnv.AgentDeploy() && !awsEnv.AgentDeployWithOperator() {
		customValues := `
datadog:
  kubelet:
    tlsVerify: false
agents:
  useHostNetwork: true
`

		k8sAgentOptions := make([]kubernetesagentparams.Option, 0)
		k8sAgentOptions = append(
			k8sAgentOptions,
			kubernetesagentparams.WithNamespace("datadog"),
			kubernetesagentparams.WithHelmValues(customValues),
			kubernetesagentparams.WithClusterName(kindCluster.ClusterName),
		)
		if fakeIntake != nil {
			k8sAgentOptions = append(
				k8sAgentOptions,
				kubernetesagentparams.WithFakeintake(fakeIntake),
			)
		}

		if awsEnv.AgentUseDualShipping() {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithDualShipping())
		}

		k8sAgentComponent, err := helm.NewKubernetesAgent(&awsEnv, awsEnv.Namer.ResourceName("datadog-agent"), kindKubeProvider, k8sAgentOptions...)

		if err != nil {
			return err
		}

		if err := k8sAgentComponent.Export(awsEnv.Ctx(), nil); err != nil {
			return err
		}

		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentComponent)
	}

	// Deploy the operator
	if awsEnv.AgentDeploy() && awsEnv.AgentDeployWithOperator() {
		operatorOpts := make([]operatorparams.Option, 0)
		operatorOpts = append(
			operatorOpts,
			operatorparams.WithNamespace("datadog"),
		)

		operatorComp, err := operator.NewOperator(&awsEnv, awsEnv.CommonNamer().ResourceName("dd-operator"), kindKubeProvider, operatorOpts...)
		if err != nil {
			return err
		}

		if err := operatorComp.Export(awsEnv.Ctx(), nil); err != nil {
			return err
		}

		ddaConfig := agentwithoperatorparams.DDAConfig{
			Name: "dda-with-operator",
			YamlConfig: `
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  global:
    kubelet:
      tlsVerify: false
`}

		ddaOptions := make([]agentwithoperatorparams.Option, 0)
		ddaOptions = append(
			ddaOptions,
			agentwithoperatorparams.WithNamespace("datadog"),
			agentwithoperatorparams.WithDDAConfig(ddaConfig),
		)

		if fakeIntake != nil {
			ddaOptions = append(
				ddaOptions,
				agentwithoperatorparams.WithFakeIntake(fakeIntake),
			)
		}

		k8sAgentWithOperatorComp, err := agent.NewDDAWithOperator(&awsEnv, awsEnv.CommonNamer().ResourceName("datadog-agent-with-operator"), kindKubeProvider, ddaOptions...)

		if err != nil {
			return err
		}

		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentWithOperatorComp)

		if err := k8sAgentWithOperatorComp.Export(awsEnv.Ctx(), nil); err != nil {
			return err
		}

	}

	// Deploy standalone dogstatsd
	if awsEnv.DogstatsdDeploy() {
		if _, err := dogstatsdstandalone.K8sAppDefinition(&awsEnv, kindKubeProvider, "dogstatsd-standalone", fakeIntake, false, kindClusterName); err != nil {
			return err
		}
	}

	// Deploy testing workload
	if awsEnv.TestingWorkloadDeploy() {
		if _, err := nginx.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-nginx", "", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
			return err
		}

		if _, err := redis.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-redis", true, dependsOnDDAgent /* for DDM */, dependsOnVPA); err != nil {
			return err
		}

		if _, err := cpustress.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-cpustress"); err != nil {
			return err
		}

		// dogstatsd clients that report to the Agent
		if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		// dogstatsd clients that report to the dogstatsd standalone deployment
		if awsEnv.DogstatsdDeploy() {
			if _, err := dogstatsd.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket, dependsOnDDAgent /* for admission */); err != nil {
				return err
			}
		}

		// for tracegen we can't find the cgroup version as it depends on the underlying version of the kernel
		if _, err := tracegen.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-tracegen"); err != nil {
			return err
		}

		if _, err := prometheus.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-prometheus"); err != nil {
			return err
		}

		if _, err := mutatedbyadmissioncontroller.K8sAppDefinition(&awsEnv, kindKubeProvider, "workload-mutated", "workload-mutated-lib-injection", dependsOnDDAgent /* for admission */); err != nil {
			return err
		}

		if _, err := etcd.K8sAppDefinition(&awsEnv, kindKubeProvider); err != nil {
			return err
		}
	}

	return nil
}
