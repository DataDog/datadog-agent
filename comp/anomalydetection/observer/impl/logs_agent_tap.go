// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"context"
	"sync/atomic"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/logs/adaptivesampling"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const anomalyDetectionLogsAgentTapEnabledKey = "anomaly_detection.logs.agent_tap.enabled"

func installLogsAgentTokenizedLogTap(obs *observerImpl, cfg config.Component, lifecycle compdef.Lifecycle, analysisEnabled, recorderEnabled, logsEnabled bool) {
	if obs == nil {
		pkglog.Infof("%s tokenized log tap not registered reason=%q", adaptivesampling.DebugLogPrefix, "observer unavailable")
		return
	}
	if cfg == nil {
		pkglog.Infof("%s tokenized log tap not registered reason=%q", adaptivesampling.DebugLogPrefix, "config unavailable")
		return
	}
	if !logsEnabled {
		pkglog.Infof("%s tokenized log tap not registered reason=%q", adaptivesampling.DebugLogPrefix, "anomaly_detection.logs.enabled is false")
		return
	}
	if !analysisEnabled && !recorderEnabled {
		pkglog.Infof("%s tokenized log tap not registered reason=%q", adaptivesampling.DebugLogPrefix, "analysis and recording are disabled")
		return
	}
	if !configBoolDefaultTrue(cfg, anomalyDetectionLogsAgentTapEnabledKey) {
		pkglog.Infof("%s tokenized log tap not registered reason=%q", adaptivesampling.DebugLogPrefix, anomalyDetectionLogsAgentTapEnabledKey+" is false")
		return
	}
	if !logsAgentCollectionEnabled(cfg) {
		pkglog.Infof("%s tokenized log tap not registered reason=%q logs_enabled=%t log_enabled=%t",
			adaptivesampling.DebugLogPrefix,
			"logs agent collection is disabled",
			cfg.GetBool("logs_enabled"),
			cfg.GetBool("log_enabled"))
		return
	}

	handle := obs.GetHandle("logs-agent-tap")
	tapCleanup := adaptivesampling.SetTokenizedLogObserver(&logsAgentTapSink{handle: handle})
	boostSink := newSamplingBoostEventSink(scorerConfigFromAgentConfig(cfg))
	boostCleanup := obs.engine.Subscribe(boostSink)
	var scorerCleanups []func()
	for _, scorer := range obs.engine.scorers {
		scorerCleanups = append(scorerCleanups, scorer.Subscribe(observerdef.AnomalyScorerConfiguration{
			Listener: boostSink,
			Filter: observerdef.ScorerEventFilter{
				ToLevels:  []observerdef.SeverityLevel{observerdef.SeverityHigh},
				Direction: observerdef.ScorerEventEscalation,
			},
		}))
	}
	pkglog.Infof("%s tokenized log tap registered analysis_enabled=%t recorder_enabled=%t logs_enabled=%t logs_agent_enabled=%t detectors=%d scorers=%d boost_sink_registered=%t scorer_high_subscriptions=%d",
		adaptivesampling.DebugLogPrefix,
		analysisEnabled,
		recorderEnabled,
		logsEnabled,
		logsAgentCollectionEnabled(cfg),
		len(obs.engine.detectors),
		len(obs.engine.scorers),
		boostCleanup != nil,
		len(scorerCleanups))

	lifecycle.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			tapCleanup()
			boostCleanup()
			for _, scorerCleanup := range scorerCleanups {
				scorerCleanup()
			}
			return nil
		},
	})
}

func logsAgentCollectionEnabled(cfg config.Component) bool {
	return cfg.GetBool("logs_enabled") || cfg.GetBool("log_enabled")
}

func configBoolDefaultTrue(cfg config.Component, key string) bool {
	return !cfg.IsConfigured(key) || cfg.GetBool(key)
}

func scorerConfigFromAgentConfig(cfg config.Component) observerdef.ScorerConfig {
	if cfg == nil {
		return DefaultScorerConfig()
	}
	if scorerCfg, ok := readScorerConfig(cfg, "anomaly_detection.detectors.anomaly_scorer.").(observerdef.ScorerConfig); ok {
		return scorerCfg
	}
	return DefaultScorerConfig()
}

type logsAgentTapSink struct {
	handle     observerdef.Handle
	debugCount atomic.Uint64
}

func (s *logsAgentTapSink) ObserveTokenizedLog(event adaptivesampling.TokenizedLogEvent) {
	if s == nil || s.handle == nil {
		return
	}
	s.logReceived(event)
	s.handle.ObserveLog(newTokenizedLogEventView(event))
}

func (s *logsAgentTapSink) logReceived(event adaptivesampling.TokenizedLogEvent) {
	count := s.debugCount.Add(1)
	if !adaptivesampling.ShouldLogDebugSample(count) {
		return
	}
	pkglog.Infof("%s observer received tokenized log observation count=%d container_id=%q pattern_hash=%q pattern=%q content=%q tag_count=%d",
		adaptivesampling.DebugLogPrefix,
		count,
		event.ContainerID,
		event.PatternHash,
		adaptivesampling.TruncateDebugString(event.Pattern, 180),
		adaptivesampling.TruncateDebugString(event.Content, 180),
		len(event.Tags))
}

type tokenizedLogEventView struct {
	event adaptivesampling.TokenizedLogEvent
	tags  []string
}

var _ observerdef.TokenizedLogView = (*tokenizedLogEventView)(nil)

func newTokenizedLogEventView(event adaptivesampling.TokenizedLogEvent) *tokenizedLogEventView {
	tags := append([]string(nil), event.Tags...)
	if event.ContainerID != "" && !containsExactTag(tags, "container_id:"+event.ContainerID) {
		tags = append(tags, "container_id:"+event.ContainerID)
	}
	return &tokenizedLogEventView{
		event: event,
		tags:  tags,
	}
}

func (v *tokenizedLogEventView) GetContent() string           { return v.event.Content }
func (v *tokenizedLogEventView) GetStatus() string            { return v.event.Status }
func (v *tokenizedLogEventView) Tags() []string               { return v.tags }
func (v *tokenizedLogEventView) GetHostname() string          { return v.event.Hostname }
func (v *tokenizedLogEventView) GetTimestampUnixMilli() int64 { return v.event.TimestampUnixMilli }
func (v *tokenizedLogEventView) GetContainerID() string       { return v.event.ContainerID }
func (v *tokenizedLogEventView) GetPattern() string           { return v.event.Pattern }
func (v *tokenizedLogEventView) GetPatternHash() string       { return v.event.PatternHash }

func containsExactTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
