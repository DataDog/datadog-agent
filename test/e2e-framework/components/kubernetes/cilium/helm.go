package cilium

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/helm"
)

type HelmValues pulumi.Map

func NewHelmInstallation(e config.Env, cluster *kubernetes.Cluster, params *Params, opts ...pulumi.ResourceOption) (*HelmComponent, error) {
	helmComponent := &HelmComponent{}
	if err := e.Ctx().RegisterComponentResource("dd:cilium", "cilium", helmComponent, opts...); err != nil {
		return nil, err
	}

	if params.hasKubeProxyReplacement() {
		params.HelmValues["k8sServiceHost"] = cluster.KubeInternalServerAddress
		params.HelmValues["k8sServicePort"] = cluster.KubeInternalServerPort
	}

	opts = append(opts, pulumi.Parent(helmComponent))
	ciliumBase, err := helm.NewInstallation(e, helm.InstallArgs{
		RepoURL:     "https://helm.cilium.io",
		ChartName:   "cilium",
		InstallName: "cilium",
		Namespace:   "kube-system",
		Values:      pulumi.Map(params.HelmValues),
		Version:     pulumi.StringPtr(params.Version),
	}, opts...)
	if err != nil {
		return nil, err
	}

	helmComponent.CiliumHelmReleaseStatus = ciliumBase.Status
	resourceOutputs := pulumi.Map{
		"CiliumBaseHelmReleaseStatus": ciliumBase.Status,
	}

	if err := e.Ctx().RegisterResourceOutputs(helmComponent, resourceOutputs); err != nil {
		return nil, err
	}

	return helmComponent, nil
}
