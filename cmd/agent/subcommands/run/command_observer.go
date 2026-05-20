// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The `python` build tag is used here as a proxy for "full agent, not IoT agent".
// IOT_AGENT_TAGS = {"jetson", "systemd", "zlib", "zstd"} does not include `python`,
// while the full AGENT_TAGS set does. This gates the observer component out of the
// IoT agent binary without requiring a dedicated build tag in tasks/build_tags.py.

//go:build python

package run

import (
	"go.uber.org/fx"

	hfrunnerfx "github.com/DataDog/datadog-agent/comp/anomalydetection/hfrunner/fx"
	logssourcefx "github.com/DataDog/datadog-agent/comp/anomalydetection/logssource/fx"
	observerfx "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/fx"
	recorderfx "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx-noop"
	reporterfx "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/fx"
)

func getObserverOptions() fx.Option {
	return fx.Options(
		observerfx.Module(),
		hfrunnerfx.Module(),
		logssourcefx.Module(),
		recorderfx.Module(),
		reporterfx.Module(),
	)
}
