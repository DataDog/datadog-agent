// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aks

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure/aks"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Params struct {
	kataNodePool bool
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	params := &Params{}
	return common.ApplyOption(params, options)
}

func WithKataNodePool() Option {
	return func(params *Params) error {
		params.kataNodePool = true
		return nil
	}
}

func NewAKSCluster(env azure.Environment, opts ...Option) (*kubeComp.Cluster, error) {
	params, err := NewParams(opts...)
	if err != nil {
		return nil, err
	}

	return components.NewComponent(&env, env.Namer.ResourceName("aks"), func(comp *kubeComp.Cluster) error {

		cluster, kubeConfig, err := aks.NewCluster(env, "aks", params.kataNodePool)
		if err != nil {
			return err
		}

		// Filling Kubernetes component from EKS cluster
		comp.ClusterName = cluster.Name
		comp.KubeConfig = kubeConfig

		// Building Kubernetes provider
		aksKubeProvider, err := kubernetes.NewProvider(env.Ctx(), env.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
			EnableServerSideApply: pulumi.BoolPtr(true),
			Kubeconfig:            utils.KubeConfigYAMLToJSON(kubeConfig),
		}, env.WithProviders(config.ProviderAzure))
		if err != nil {
			return err
		}
		comp.KubeProvider = aksKubeProvider

		return nil
	})
}
