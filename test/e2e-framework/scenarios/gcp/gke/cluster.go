// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gke

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp/gke"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed workloadallowlist.yaml
var autopilotAllowListYAML string

//go:embed workloadcsiallowlist.yaml
var workloadCSIAllowListYAML string

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
				YAML: []string{autopilotAllowListYAML, workloadCSIAllowListYAML},
			}, pulumi.Provider(gkeKubeProvider), env.WithProviders(config.ProviderGCP))
			if err != nil {
				return err
			}
		}

		return nil
	})
}
