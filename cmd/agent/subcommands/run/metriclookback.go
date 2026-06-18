// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"
	"time"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/checkloader"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
	lookbacktrigger "github.com/DataDog/datadog-agent/pkg/collector/metriclookback/trigger"
	collectorrunner "github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func newMetricLookbackRetention(cfg config.Component, hostname string) aggregator.LookbackRetention {
	return metriclookback.NewRetentionFromConfig(cfg, hostname)
}

func newMetricLookbackTrigger(cfg config.Component, logger log.Component, dump aggregator.LookbackDumper) aggregator.LookbackTrigger {
	if !cfg.GetBool("metric_lookback.trigger.enabled") {
		return nil
	}
	if !cfg.GetBool("metric_lookback.enabled") {
		logger.Warn("metric_lookback.trigger.enabled is set but metric_lookback.enabled is false; trigger inactive")
		return nil
	}

	metricName := cfg.GetString("metric_lookback.trigger.metric_name")
	watcher := lookbacktrigger.New(lookbacktrigger.Config{
		MetricName:   metricName,
		Threshold:    cfg.GetFloat64("metric_lookback.trigger.threshold"),
		Alpha:        cfg.GetFloat64("metric_lookback.trigger.ewma_alpha"),
		Cooldown:     cfg.GetDuration("metric_lookback.trigger.cooldown"),
		PreWindow:    cfg.GetDuration("metric_lookback.trigger.pre_window"),
		PostWindow:   cfg.GetDuration("metric_lookback.trigger.post_window"),
		DumpInterval: cfg.GetDuration("metric_lookback.trigger.dump_interval"),
		SendDelay:    cfg.GetDuration("metric_lookback.trigger.send_delay"),
	}, func(from, to time.Time) (int, error) {
		count, err := dump(from, to)
		if err != nil {
			logger.Warnf("lookback trigger dump failed for %q window [%s, %s]: %v", metricName, from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano), err)
			return 0, err
		}
		logger.Infof("lookback trigger on %q dumped %d series for window [%s, %s]", metricName, count, from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano))
		return count, nil
	})
	if watcher == nil {
		logger.Warn("metric_lookback.trigger.enabled is set but metric_lookback.trigger.metric_name is empty; trigger inactive")
	}
	return watcher
}

type lookbackSenderManagerProvider interface {
	LookbackSenderManager(context.Context) sender.SenderManager
}

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
	loaderCatalog := loaders.LoaderCatalog(demux, logReceiver, tagger, filterStore)
	loader := checkloader.New(loaderCatalog, demux, noopShadowLoaderErrorRecorder{})
	shadowScheduler := metriclookback.NewShadowScheduler(metriclookback.ShadowSchedulerOptions{
		Loader: loader,
		NewSenderManager: func(ctx context.Context) sender.SenderManager {
			if provider, ok := demux.(lookbackSenderManagerProvider); ok {
				if manager := provider.LookbackSenderManager(ctx); manager != nil {
					return manager
				}
			}
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
		ChecksToShadow:      cfg.GetStringSlice("metric_lookback.enabled_checks"),
		ResolveLoader: func(config integration.Config, instance integration.Data) (string, bool) {
			loaderName, resolved, err := loader.ResolveEffectiveLoader(config, instance)
			return loaderName, resolved && err == nil
		},
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
