// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package privatebundles

import (
	com_datadoghq_datadog_agentactions "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/agentactions"
	com_datadoghq_kubernetes_apiextensions "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apiextensions"
	com_datadoghq_kubernetes_apps "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apps"
	com_datadoghq_kubernetes_batch "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/batch"
	com_datadoghq_kubernetes_core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/core"
	com_datadoghq_kubernetes_customresources "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/customresources"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Registry struct {
	Bundles map[string]types.Bundle
}

func NewRegistry() *Registry {
	return &Registry{
		Bundles: map[string]types.Bundle{
			"com.datadoghq.agentactions":               com_datadoghq_datadog_agentactions.NewDatadogAgentActions(),
			"com.datadoghq.kubernetes.apiextensions":   com_datadoghq_kubernetes_apiextensions.NewKubernetesApiExtensions(),
			"com.datadoghq.kubernetes.apps":            com_datadoghq_kubernetes_apps.NewKubernetesApps(),
			"com.datadoghq.kubernetes.batch":           com_datadoghq_kubernetes_batch.NewKubernetesBatch(),
			"com.datadoghq.kubernetes.core":            com_datadoghq_kubernetes_core.NewKubernetesCore(),
			"com.datadoghq.kubernetes.customresources": com_datadoghq_kubernetes_customresources.NewKubernetesCustomResources(),
		},
	}
}

func (r *Registry) GetBundle(fqn string) types.Bundle {
	return r.Bundles[fqn]
}
