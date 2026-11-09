// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package run

import (
	"go.uber.org/fx"

	notableeventsfx "github.com/DataDog/datadog-agent/comp/notableevents/fx"
	softwareinventoryfx "github.com/DataDog/datadog-agent/comp/softwareinventory/fx"
)

// getPlatformModules returns the Darwin-specific fx modules for the Agent run command.
func getPlatformModules() fx.Option {
	return fx.Options(
		softwareinventoryfx.Module(),
		notableeventsfx.Module(),
	)
}
