// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/checkloader"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
	collectorrunner "github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func registerMetricLookbackScheduler(
	ac autodiscovery.Component,
	cfg config.Component,
	demux demultiplexer.Component,
	logReceiver option.Option[integrations.Component],
	tagger tagger.Component,
	filterStore workloadfilter.Component,
	haAgent haagent.Component,
	healthplatform healthplatform.Component,
	hostname hostnameinterface.Component,
) {
	loader := checkloader.New(loaders.LoaderCatalog(demux, logReceiver, tagger, filterStore), demux, noopShadowLoaderErrorRecorder{})
	shadowScheduler := metriclookback.NewShadowScheduler(metriclookback.ShadowSchedulerOptions{
		Loader: loader,
		NewSenderManager: func(ctx context.Context) sender.SenderManager {
			return lookbacksender.NewSenderManager(ctx, hostname.GetSafe(ctx), noopLookbackWriter{}, nil)
		},
		NewRunner: func(scheduled collectorrunner.ScheduledChecks) metriclookback.ShadowRunner {
			r := collectorrunner.NewRunnerWithOptions(
				demux,
				haAgent,
				healthplatform,
				collectorrunner.Options{StatusEmitter: noopShadowStatusEmitter{}},
			)
			r.SetScheduler(scheduled)
			return r
		},
	})

	ac.AddScheduler("metric_lookback", metriclookback.NewAutoConfigShadowAdapter(metriclookback.Options{
		ShadowChecksEnabled: cfg.GetBool("metric_lookback.enabled"),
		ChecksToShadow:      cfg.GetStringSlice("metric_lookback.checks"),
	}, shadowScheduler), true)
}

type noopLookbackWriter struct{}

func (noopLookbackWriter) Append(context.Context, checkid.ID, []metrics.MetricSample) error {
	return nil
}

type noopShadowStatusEmitter struct{}

func (noopShadowStatusEmitter) Emit(context.Context, check.Check, error, []error) {}

type noopShadowLoaderErrorRecorder struct{}

func (noopShadowLoaderErrorRecorder) SetLoaderError(_, _, _ string) {}

func (noopShadowLoaderErrorRecorder) RemoveLoaderErrors(_ string) {}
