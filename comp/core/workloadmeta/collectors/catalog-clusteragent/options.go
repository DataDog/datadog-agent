// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package catalog

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/kubeapiserver"
)

func getCollectorOptions() []fx.Option {
	return []fx.Option{
		kubeapiserver.GetFxOptions(),
	}
}
