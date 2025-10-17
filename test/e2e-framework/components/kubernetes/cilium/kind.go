package cilium

import (
	"embed"
	"fmt"
	"net/url"
	"strings"
	"text/template"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/components/remote"
)

//go:embed kind-cilium-cluster.yaml
var kindCilumClusterFS embed.FS

func kindKubeClusterConfigFromCiliumParams(params *Params) (string, error) {
	o := struct {
		KubeProxyReplacement bool
	}{
		KubeProxyReplacement: params.hasKubeProxyReplacement(),
	}

	kindCiliumClusterTemplate, err := template.ParseFS(kindCilumClusterFS, "kind-cilium-cluster.yaml")
	if err != nil {
		return "", err
	}

	var kindCilumClusterConfig strings.Builder
	if err = kindCiliumClusterTemplate.Execute(&kindCilumClusterConfig, o); err != nil {
		return "", err
	}

	return kindCilumClusterConfig.String(), nil
}

func NewKindCluster(env config.Env, vm *remote.Host, name string, kubeVersion string, ciliumOpts []Option, opts ...pulumi.ResourceOption) (*kubernetes.Cluster, error) {
	params, err := NewParams(ciliumOpts...)
	if err != nil {
		return nil, fmt.Errorf("could not create cilium params from opts: %w", err)
	}

	clusterConfig, err := kindKubeClusterConfigFromCiliumParams(params)
	if err != nil {
		return nil, err
	}

	cluster, err := kubernetes.NewKindClusterWithConfig(env, vm, name, kubeVersion, clusterConfig, opts...)
	if err != nil {
		return nil, err
	}

	if params.hasKubeProxyReplacement() {
		runner := vm.OS.Runner()
		kindClusterName := env.CommonNamer().DisplayName(49) // We can have some issues if the name is longer than 50 characters
		kubeConfigInternalCmd, err := runner.Command(
			env.CommonNamer().ResourceName("kube-kubeconfig-internal"),
			&command.Args{
				Create: pulumi.Sprintf("kind get kubeconfig --name %s --internal", kindClusterName),
			},
			utils.MergeOptions(opts, utils.PulumiDependsOn(cluster))...,
		)
		if err != nil {
			return nil, err
		}

		hostPort := kubeConfigInternalCmd.StdoutOutput().ApplyT(
			func(v string) ([]string, error) {
				out := map[string]interface{}{}
				if err := yaml.Unmarshal([]byte(v), out); err != nil {
					return nil, fmt.Errorf("error unmarshaling output of kubeconfig: %w", err)
				}

				clusters := out["clusters"].([]interface{})
				cluster := clusters[0].(map[string]interface{})["cluster"]
				server := cluster.(map[string]interface{})["server"].(string)
				u, err := url.Parse(server)
				if err != nil {
					return nil, fmt.Errorf("could not parse server address %s: %w", server, err)
				}

				return []string{u.Hostname(), u.Port()}, nil
			},
		).(pulumi.StringArrayOutput)

		cluster.KubeInternalServerAddress = hostPort.Index(pulumi.Int(0))
		cluster.KubeInternalServerPort = hostPort.Index(pulumi.Int(1))
	}

	return cluster, nil
}
