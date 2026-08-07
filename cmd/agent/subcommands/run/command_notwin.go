// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package run

import (
	"go.uber.org/fx"

	softwareinventoryfx "github.com/DataDog/datadog-agent/comp/softwareinventory/fx"
)

func getPlatformModules() fx.Option {
	return fx.Options(
		softwareinventoryfx.Module(),
	)
}
