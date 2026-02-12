// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !test

package run

import (
	"go.uber.org/fx"

	taggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx"
)

func getTaggerModule() fx.Option {
	return taggerfx.Module()
}
