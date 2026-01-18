// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package cilium

import (
	"embed"
	"fmt"
	"net/url"
	"strings"
	"text/template"

	"github.com/Masterminds/semver/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
)

//go:embed kind-cilium-cluster.yaml
var kindCilumClusterFS embed.FS

//go:embed kind-cluster-v1.35+.yaml
var kindCiliumV135ClusterFS embed.FS

//go:embed hosts.toml
var containerdDockerioHostConfig string

func kindKubeClusterConfigFromCiliumParams(params *Params, kubeVersion string) (string, error) {
	o := struct {
		KubeProxyReplacement bool
	}{
		KubeProxyReplacement: params.hasKubeProxyReplacement(),
	}

	var kindCiliumCluster embed.FS = kindCilumClusterFS
	if index := strings.Index(kubeVersion, "@"); index != -1 {
		kubeVersion = kubeVersion[:index]
	}

	if semver.MustParse(kubeVersion).GreaterThanEqual(semver.MustParse("v1.35.0")) {
		kindCiliumCluster = kindCiliumV135ClusterFS
	}

	kindCiliumClusterTemplate, err := template.ParseFS(kindCiliumCluster, "kind-cilium-cluster.yaml")
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

	clusterConfig, err := kindKubeClusterConfigFromCiliumParams(params, kubeVersion)
	if err != nil {
		return nil, err
	}

	cluster, err := kubernetes.NewKindClusterWithConfig(env, vm, name, kubeVersion, clusterConfig, containerdDockerioHostConfig, opts...)
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
