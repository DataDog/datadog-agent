// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"github.com/DataDog/datadog-agent/comp/workloadmeta"
	"go.uber.org/fx"
)

type WorkloadCollector struct {
	fx.Out

	Collector workloadmeta.Collector `group:"workloadmeta"`
}
