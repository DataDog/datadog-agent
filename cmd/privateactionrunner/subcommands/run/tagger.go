// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !test

package run

import (
	"go.uber.org/fx"

	taggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
)

func getTaggerModule() fx.Option {
	return fx.Options(
		workloadmetafx.Module(workloadmeta.NewParams()),
		taggerfx.Module(),
	)
}
