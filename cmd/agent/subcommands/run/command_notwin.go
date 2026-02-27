// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package run

import (
	logondurationfx "github.com/DataDog/datadog-agent/comp/logonduration/fx"
	"go.uber.org/fx"
)

func getPlatformModules() fx.Option {
	return fx.Options(
		logondurationfx.Module(),
	)
}
