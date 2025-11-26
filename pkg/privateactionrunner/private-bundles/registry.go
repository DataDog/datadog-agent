// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package privatebundles

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Registry struct {
	Bundles map[string]types.Bundle
}

func NewRegistry() *Registry {
	return &Registry{
		Bundles: map[string]types.Bundle{},
	}
}

func (r *Registry) GetBundle(fqn string) types.Bundle {
	return r.Bundles[fqn]
}
