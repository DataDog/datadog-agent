// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package productcomposition

import (
	"go.uber.org/fx"

	logssourcefx "github.com/DataDog/datadog-agent/comp/anomalydetection/logssource/fx"
	observerfx "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/fx"
	recorderfx "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx"
	reporterfx "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/fx"
)

func anomalyDetectionOptions() []fx.Option {
	return []fx.Option{
		observerfx.Module(),
		logssourcefx.Module(),
		recorderfx.Module(),
		reporterfx.Module(),
	}
}
