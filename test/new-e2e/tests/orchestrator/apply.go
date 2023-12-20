package orchestrator

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/dogstatsd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/prometheus"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/redis"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	ddfakeintake "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	localKubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	awsResources "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"

	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func ApplyNoReturn(ctx *pulumi.Context) error {
	_, _, err := Apply(ctx)
	return err
}

func Apply(ctx *pulumi.Context) (*ec2vm.EC2UnixVM, *awsResources.Environment, error) {
	vm, err := ec2vm.NewUnixEc2VM(ctx)
	if err != nil {
		return nil, nil, err
	}
	awsEnv := vm.Infra.GetAwsEnvironment()

	kubeConfigCommand, kubeConfig, err := localKubernetes.NewKindCluster(vm.UnixVM, awsEnv.CommonNamer.ResourceName("kind"), "amd64", awsEnv.KubernetesVersion())
	if err != nil {
		return nil, nil, err
	}

	// Export cluster’s properties
	ctx.Export("kubeconfig", kubeConfig)

	// Building Kubernetes provider
	kindKubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.BoolPtr(true),
		Kubeconfig:            kubeConfig,
	}, utils.PulumiDependsOn(kubeConfigCommand))
	if err != nil {
		return nil, nil, err
	}

	var dependsOnCrd pulumi.ResourceOption

	var fakeIntake *ddfakeintake.ConnectionExporter
	if awsEnv.GetCommonEnvironment().AgentUseFakeintake() {
		if fakeIntake, err = aws.NewEcsFakeintake(awsEnv); err != nil {
			return nil, nil, err
		}
	}

	clusterName := ctx.Stack()

	// Deploy the agent
	if awsEnv.AgentDeploy() {
		customValues := fmt.Sprintf(`
datadog:
  kubelet:
    tlsVerify: false
  clusterName: "%s"
agents:
  useHostNetwork: true
`, clusterName)

		helmComponent, err := agent.NewHelmInstallation(*awsEnv.CommonEnvironment, agent.HelmInstallationArgs{
			KubeProvider: kindKubeProvider,
			Namespace:    "datadog",
			ValuesYAML: pulumi.AssetOrArchiveArray{
				pulumi.NewStringAsset(customValues),
			},
			Fakeintake: fakeIntake,
		}, nil)
		if err != nil {
			return nil, nil, err
		}

		ctx.Export("kube-cluster-name", pulumi.String(clusterName))
		ctx.Export("agent-linux-helm-install-name", helmComponent.LinuxHelmReleaseName)
		ctx.Export("agent-linux-helm-install-status", helmComponent.LinuxHelmReleaseStatus)

		dependsOnCrd = utils.PulumiDependsOn(helmComponent)
	}

	// Deploy standalone dogstatsd
	if awsEnv.DogstatsdDeploy() {
		if _, err := dogstatsdstandalone.K8sAppDefinition(*awsEnv.CommonEnvironment, kindKubeProvider, "dogstatsd-standalone", fakeIntake, false, clusterName); err != nil {
			return nil, nil, err
		}
	}

	// Deploy testing workload
	if awsEnv.TestingWorkloadDeploy() {
		if _, err := nginx.K8sAppDefinition(*awsEnv.CommonEnvironment, kindKubeProvider, "workload-nginx", dependsOnCrd); err != nil {
			return nil, nil, err
		}

		if _, err := redis.K8sAppDefinition(*awsEnv.CommonEnvironment, kindKubeProvider, "workload-redis", dependsOnCrd); err != nil {
			return nil, nil, err
		}

		if _, err := cpustress.K8sAppDefinition(*awsEnv.CommonEnvironment, kindKubeProvider, "workload-cpustress"); err != nil {
			return nil, nil, err
		}

		// dogstatsd clients that report to the Agent
		if _, err := dogstatsd.K8sAppDefinition(*awsEnv.CommonEnvironment, kindKubeProvider, "workload-dogstatsd", 8125, "/var/run/datadog/dsd.socket"); err != nil {
			return nil, nil, err
		}

		// dogstatsd clients that report to the dogstatsd standalone deployment
		if _, err := dogstatsd.K8sAppDefinition(*awsEnv.CommonEnvironment, kindKubeProvider, "workload-dogstatsd-standalone", dogstatsdstandalone.HostPort, dogstatsdstandalone.Socket); err != nil {
			return nil, nil, err
		}

		if _, err := prometheus.K8sAppDefinition(*awsEnv.CommonEnvironment, kindKubeProvider, "workload-prometheus"); err != nil {
			return nil, nil, err
		}
	}

	return vm, &awsEnv, nil
}
