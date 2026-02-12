// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// This file provides the bundle registry used inside the DD cluster agent.
package privatebundles

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	com_datadoghq_ddagent "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/ddagent"
	com_datadoghq_http "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/http"
	com_datadoghq_jenkins "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/jenkins"
	com_datadoghq_kubernetes_apiextensions "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apiextensions"
	com_datadoghq_kubernetes_apps "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apps"
	com_datadoghq_kubernetes_batch "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/batch"
	com_datadoghq_kubernetes_core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/core"
	com_datadoghq_kubernetes_customresources "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/customresources"
	com_datadoghq_script "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/script"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Registry struct {
	Bundles map[string]types.Bundle
}

func NewRegistry(configuration *config.Config) *Registry {
	return &Registry{
		Bundles: map[string]types.Bundle{
			"com.datadoghq.ddagent":                    com_datadoghq_ddagent.NewAgentActions(),
			"com.datadoghq.http":                       com_datadoghq_http.NewHttpBundle(configuration),
			"com.datadoghq.jenkins":                    com_datadoghq_jenkins.NewJenkins(configuration),
			"com.datadoghq.kubernetes.apiextensions":   com_datadoghq_kubernetes_apiextensions.NewKubernetesApiExtensions(),
			"com.datadoghq.kubernetes.apps":            com_datadoghq_kubernetes_apps.NewKubernetesApps(),
			"com.datadoghq.kubernetes.batch":           com_datadoghq_kubernetes_batch.NewKubernetesBatch(),
			"com.datadoghq.kubernetes.core":            com_datadoghq_kubernetes_core.NewKubernetesCore(),
			"com.datadoghq.kubernetes.customresources": com_datadoghq_kubernetes_customresources.NewKubernetesCustomResources(),
			"com.datadoghq.script":                     com_datadoghq_script.NewScript(),
		},
	}
}

func (r *Registry) GetBundle(fqn string) types.Bundle {
	return r.Bundles[fqn]
}
