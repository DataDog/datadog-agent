// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package productcomposition

import (
	"go.uber.org/fx"

	doqueryactionsfx "github.com/DataDog/datadog-agent/comp/dataobs/queryactions/fx"
)

func dataObservabilityOptions() []fx.Option {
	return []fx.Option{
		doqueryactionsfx.Module(),
	}
}
