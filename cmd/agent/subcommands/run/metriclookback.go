// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python || test

package run

import (
	"go.uber.org/fx"

	metriclookbackfx "github.com/DataDog/datadog-agent/comp/metriclookback/fx"
)

func metriclookbackModule() fx.Option {
	return metriclookbackfx.Module()
}
