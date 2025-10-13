package operator

import (
	compkubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
)

func NewOperator(e config.Env, resourceName string, kubeProvider *kubernetes.Provider, options ...operatorparams.Option) (*Operator, error) {
	return components.NewComponent(e, resourceName, func(comp *Operator) error {
		params, err := operatorparams.NewParams(e, options...)
		if err != nil {
			return err
		}
		pulumiResourceOptions := append(params.PulumiResourceOptions, pulumi.Parent(comp))

		release, err := NewHelmInstallation(e, HelmInstallationArgs{
			KubeProvider:          kubeProvider,
			Namespace:             params.Namespace,
			ChartPath:             params.HelmChartPath,
			RepoURL:               params.HelmRepoURL,
			ValuesYAML:            params.HelmValues,
			OperatorFullImagePath: params.OperatorFullImagePath,
		}, pulumiResourceOptions...)
		if err != nil {
			return err
		}

		comp.Operator, err = compkubernetes.NewKubernetesObjRef(e, "datadog-operator", params.Namespace, "Pod", release.LinuxHelmReleaseStatus.AppVersion().Elem(), release.LinuxHelmReleaseStatus.Version().Elem(), map[string]string{"app.kubernetes.io/name": "datadog-operator"})

		if err != nil {
			return err
		}

		return nil
	})
}
