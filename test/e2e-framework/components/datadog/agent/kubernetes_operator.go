// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dda"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewDDAWithOperator(e config.Env, resourceName string, kubeProvider *kubernetes.Provider, ddaOptions ...agentwithoperatorparams.Option) (*KubernetesAgent, error) {
	return components.NewComponent(e, resourceName, func(comp *KubernetesAgent) error {
		ddaParams, err := agentwithoperatorparams.NewParams(ddaOptions...)
		if err != nil {
			return err
		}

		ddaParams.PulumiResourceOptions = append(ddaParams.PulumiResourceOptions, pulumi.Parent(comp))

		_, err = dda.K8sAppDefinition(e, kubeProvider, ddaParams, ddaParams.PulumiResourceOptions...)

		if err != nil {
			return err
		}

		baseName := "dda-with-operator-linux"

		comp.LinuxNodeAgent, err = componentskube.NewKubernetesObjRef(e, baseName+"-nodeAgent", ddaParams.Namespace, "Pod", pulumi.String("").ToStringOutput(), pulumi.String("datadoghq/v2alpha1").ToStringOutput(), map[string]string{"app.kubernetes.io/instance": ddaParams.DDAConfig.Name + "-agent"})

		if err != nil {
			return err
		}

		comp.LinuxClusterAgent, err = componentskube.NewKubernetesObjRef(e, baseName+"-clusterAgent", ddaParams.Namespace, "Pod", pulumi.String("").ToStringOutput(), pulumi.String("datadoghq/v2alpha1").ToStringOutput(), map[string]string{
			"app.kubernetes.io/instance": ddaParams.DDAConfig.Name + "-cluster-agent",
		})

		if err != nil {
			return err
		}

		comp.LinuxClusterChecks, err = componentskube.NewKubernetesObjRef(e, baseName+"-clusterChecks", ddaParams.Namespace, "Pod", pulumi.String("").ToStringOutput(), pulumi.String("datadoghq/v2alpha1").ToStringOutput(), map[string]string{
			"app.kubernetes.io/instance": ddaParams.DDAConfig.Name + "-cluster-checks-runner",
		})

		if err != nil {
			return err
		}

		return nil
	})
}
