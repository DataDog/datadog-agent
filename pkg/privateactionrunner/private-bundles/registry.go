// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package privatebundles provides a registry for managing private action bundles.
package privatebundles

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	com_datadoghq_kubernetes_core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/kubernetes/core"
	com_datadoghq_script "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/script"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// Registry manages a collection of private action bundles.
type Registry struct {
	Bundles map[string]types.Bundle
}

// NewRegistry creates a new Registry instance with default bundles.
func NewRegistry(_ *config.Config) *Registry {
	return &Registry{
		Bundles: map[string]types.Bundle{
			"com.datadoghq.kubernetes.core": com_datadoghq_kubernetes_core.NewKubernetesCore(),
			"com.datadoghq.script":          com_datadoghq_script.NewScript(),
		},
	}
}

// GetBundle returns the bundle with the specified fully qualified name.
func (r *Registry) GetBundle(fqn string) types.Bundle {
	return r.Bundles[fqn]
}
