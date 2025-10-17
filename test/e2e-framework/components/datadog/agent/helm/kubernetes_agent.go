package helm

import (
	componentskube "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
)

func NewKubernetesAgent(e config.Env, resourceName string, kubeProvider *kubernetes.Provider, options ...kubernetesagentparams.Option) (*agent.KubernetesAgent, error) {
	return components.NewComponent(e, resourceName, func(comp *agent.KubernetesAgent) error {
		params, err := kubernetesagentparams.NewParams(e, options...)
		if err != nil {
			return err
		}
		comp.FIPSEnabled = pulumi.Bool(e.AgentFIPS() || params.FIPS).ToBoolOutput()

		pulumiResourceOptions := append(params.PulumiResourceOptions, pulumi.Parent(comp))

		helmComponent, err := agent.NewHelmInstallation(e, agent.HelmInstallationArgs{
			KubeProvider:                   kubeProvider,
			DeployWindows:                  params.DeployWindows,
			Namespace:                      params.Namespace,
			ChartPath:                      params.HelmChartPath,
			RepoURL:                        params.HelmRepoURL,
			ValuesYAML:                     params.HelmValues,
			Fakeintake:                     params.FakeIntake,
			AgentFullImagePath:             params.AgentFullImagePath,
			ClusterAgentFullImagePath:      params.ClusterAgentFullImagePath,
			DualShipping:                   params.DualShipping,
			DisableLogsContainerCollectAll: params.DisableLogsContainerCollectAll,
			OTelAgent:                      params.OTelAgent,
			OTelConfig:                     params.OTelConfig,
			GKEAutopilot:                   params.GKEAutopilot,
			FIPS:                           params.FIPS,
			JMX:                            params.JMX,
		}, pulumiResourceOptions...)
		if err != nil {
			return err
		}

		comp.ClusterAgentToken = helmComponent.ClusterAgentToken

		platform := "linux"
		appVersion := helmComponent.LinuxHelmReleaseStatus.AppVersion().Elem()
		version := helmComponent.LinuxHelmReleaseStatus.Version().Elem()

		baseName := "dda-" + platform

		comp.LinuxNodeAgent, err = componentskube.NewKubernetesObjRef(e, baseName+"-nodeAgent", params.Namespace, "Pod", appVersion, version, map[string]string{
			"app": baseName + "-datadog",
		})

		if err != nil {
			return err
		}

		comp.LinuxClusterAgent, err = componentskube.NewKubernetesObjRef(e, baseName+"-clusterAgent", params.Namespace, "Pod", appVersion, version, map[string]string{
			"app": baseName + "-datadog-cluster-agent",
		})

		if err != nil {
			return err
		}

		comp.LinuxClusterChecks, err = componentskube.NewKubernetesObjRef(e, baseName+"-clusterChecks", params.Namespace, "Pod", appVersion, version, map[string]string{
			"app": baseName + "-datadog-clusterchecks",
		})

		if params.DeployWindows {
			platform = "windows"
			appVersion = helmComponent.WindowsHelmReleaseStatus.AppVersion().Elem()
			version = helmComponent.WindowsHelmReleaseStatus.Version().Elem()

			baseName = "dda-" + platform

			comp.WindowsNodeAgent, err = componentskube.NewKubernetesObjRef(e, baseName+"-nodeAgent", params.Namespace, "Pod", appVersion, version, map[string]string{
				"app": baseName + "-datadog",
			})
			if err != nil {
				return err
			}

			comp.WindowsClusterAgent, err = componentskube.NewKubernetesObjRef(e, baseName+"-clusterAgent", params.Namespace, "Pod", appVersion, version, map[string]string{
				"app": baseName + "-datadog-cluster-agent",
			})
			if err != nil {
				return err
			}

			comp.WindowsClusterChecks, err = componentskube.NewKubernetesObjRef(e, baseName+"-clusterChecks", params.Namespace, "Pod", appVersion, version, map[string]string{
				"app": baseName + "-datadog-clusterchecks",
			})
			if err != nil {
				return err
			}
		}

		if err != nil {
			return err
		}

		return nil
	})
}
