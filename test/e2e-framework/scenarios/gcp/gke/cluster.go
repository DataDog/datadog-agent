package gke

import (
	_ "embed"

	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/gcp"
	"github.com/DataDog/test-infra-definitions/resources/gcp/gke"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed workloadallowlist.yaml
var autopilotAllowListYAML string

type Params struct {
	autopilot bool
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	params := &Params{}
	return common.ApplyOption(params, options)
}

func WithAutopilot() Option {
	return func(params *Params) error {
		params.autopilot = true
		return nil
	}
}

func NewGKECluster(env gcp.Environment, opts ...Option) (*kubeComp.Cluster, error) {
	params, err := NewParams(opts...)
	if err != nil {
		return nil, err
	}

	return components.NewComponent(&env, env.Namer.ResourceName("gke"), func(comp *kubeComp.Cluster) error {
		cluster, kubeConfig, err := gke.NewCluster(env, "gke", params.autopilot)
		if err != nil {
			return err
		}

		comp.ClusterName = cluster.Name
		comp.KubeConfig = kubeConfig

		// Building Kubernetes provider
		gkeKubeProvider, err := kubernetes.NewProvider(env.Ctx(), env.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
			EnableServerSideApply: pulumi.BoolPtr(true),
			Kubeconfig:            utils.KubeConfigYAMLToJSON(kubeConfig),
		}, env.WithProviders(config.ProviderGCP))
		if err != nil {
			return err
		}
		comp.KubeProvider = gkeKubeProvider

		// Apply allowlist if autopilot is enabled
		if params.autopilot {
			_, err = yaml.NewConfigGroup(env.Ctx(), env.Namer.ResourceName("autopilot-allowlist"), &yaml.ConfigGroupArgs{
				YAML: []string{autopilotAllowListYAML},
			}, pulumi.Provider(gkeKubeProvider), env.WithProviders(config.ProviderGCP))
			if err != nil {
				return err
			}
		}

		return nil
	})
}
