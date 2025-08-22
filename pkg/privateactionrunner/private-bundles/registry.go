// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package privatebundles

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	com_datadoghq_kubernetes_core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/kubernetes/core"
	com_datadoghq_script "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/script"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Registry struct {
	Bundles map[string]types.Bundle
}

func NewRegistry(configuration *config.Config) *Registry {
	return &Registry{
		Bundles: map[string]types.Bundle{
			"com.datadoghq.kubernetes.core": com_datadoghq_kubernetes_core.NewKubernetesCore(),
			"com.datadoghq.script":          com_datadoghq_script.NewScript(),
		},
	}
}

func (r *Registry) GetBundle(fqn string) types.Bundle {
	return r.Bundles[fqn]
}
