// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ndm

package run

import (
	"go.uber.org/fx"

	ndmrcconfigfx "github.com/DataDog/datadog-agent/comp/ndm/rcconfig/fx"
	ndmdiscoveryfx "github.com/DataDog/datadog-agent/comp/ndmdiscovery/fx"
)

// ndmRemoteConfigModules registers the NDM Remote Configuration provider and
// the discovery component it schedules into. They go together: the provider
// hands ranges to the component.
func ndmRemoteConfigModules() fx.Option {
	return fx.Options(
		ndmrcconfigfx.Module(),
		ndmdiscoveryfx.Module(),
	)
}
