package privatebundles

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	com_datadoghq_kubernetes_core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/kubernetes/core"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Registry struct {
	Bundles map[string]types.Bundle
}

func NewRegistry(configuration *config.Config) *Registry {
	return &Registry{
		Bundles: map[string]types.Bundle{
			"com.datadoghq.kubernetes.core": com_datadoghq_kubernetes_core.NewKubernetesCore(),
		},
	}
}

func (r *Registry) GetBundle(fqn string) types.Bundle {
	return r.Bundles[fqn]
}
