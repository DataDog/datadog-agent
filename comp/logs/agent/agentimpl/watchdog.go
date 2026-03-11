// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"context"
	"errors"
	"runtime"
	"time"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	logsmetrics "github.com/DataDog/datadog-agent/pkg/logs/metrics"
	logsstatus "github.com/DataDog/datadog-agent/pkg/logs/status"
)

const (
	autoProfileConfigKey = "logs_config.logs_agent_profile"

	autoProfileWarmup        = 60 * time.Second
	autoProfileEvalInterval  = 10 * time.Second
	autoProfileCooldown      = 10 * time.Minute
	autoProfileMaxRestartsHr = 3

	autoSaturationHighThreshold = 0.70
	autoSaturationLowThreshold  = 0.30

	autoReasonStrategy     = "strategy_saturated"
	autoReasonSender       = "sender_saturated"
	autoReasonProcess      = "processor_saturated"
	autoReasonRecovered    = "recovered"
	autoReasonNoBottleneck = "no_bottleneck"

	autoDefaultBatchMaxConcurrentSend = 0
	autoDefaultZstdCompressionLevel   = 1
	autoDefaultGzipCompressionLevel   = 6
)

var autoProfileControlledKeys = []string{
	"logs_config.pipelines",
	"logs_config.batch_max_concurrent_send",
	"logs_config.use_compression",
	"logs_config.compression_kind",
	"logs_config.zstd_compression_level",
	"logs_config.compression_level",
}

type autoProfileRuntimeValues struct {
	pipelines              int
	batchMaxConcurrentSend int
	useCompression         bool
	compressionKind        string
	zstdCompressionLevel   int
	gzipCompressionLevel   int
}

type autoProfileLimits struct {
	baselinePipelines   int
	maxPipelines        int
	baselineConcurrency int
}

type autoProfileAction struct {
	name    string
	reason  string
	changes map[string]interface{}
}

type autoProfileWatchdog struct {
	agent *logAgent

	cancel context.CancelFunc
	done   chan struct{}

	startTime     time.Time
	cooldownUntil time.Time
	applyHistory  []time.Time
}

func newAutoProfileWatchdog(agent *logAgent) *autoProfileWatchdog {
	return &autoProfileWatchdog{
		agent:        agent,
		done:         make(chan struct{}),
		startTime:    time.Now(),
		applyHistory: make([]time.Time, 0, autoProfileMaxRestartsHr),
	}
}

func (w *autoProfileWatchdog) start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go w.run(ctx)
}

func (w *autoProfileWatchdog) stop() {
	if w == nil || w.cancel == nil {
		return
	}
	w.cancel()
	<-w.done
}

func (w *autoProfileWatchdog) run(ctx context.Context) {
	defer close(w.done)

	ticker := time.NewTicker(autoProfileEvalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.evaluateAndApply()
		case <-ctx.Done():
			return
		}
	}
}

func (w *autoProfileWatchdog) evaluateAndApply() {
	now := time.Now()

	action, skipReason := w.decide(now)
	if skipReason != "" {
		logsmetrics.TlmAutoProfileSkipped.Inc(skipReason)
		logsmetrics.GlobalAutoProfileStatus.RecordDecision("skipped", skipReason)
		return
	}

	logsmetrics.TlmAutoProfileDecision.Inc(action.name, action.reason)
	logsmetrics.GlobalAutoProfileStatus.RecordDecision(action.name, action.reason)

	if len(action.changes) == 0 {
		logsmetrics.TlmAutoProfileSkipped.Inc("no_change")
		return
	}

	if err := w.apply(action, now); err != nil {
		w.agent.log.Warnf("Auto profile watchdog apply failed: %v", err)
	}
}

func (w *autoProfileWatchdog) decide(now time.Time) (autoProfileAction, string) {
	if !logsconfig.IsAutoProfileEnabled(w.agent.config) {
		return autoProfileAction{}, "disabled"
	}
	if now.Sub(w.startTime) < autoProfileWarmup {
		return autoProfileAction{}, "warmup"
	}
	if now.Before(w.cooldownUntil) {
		logsmetrics.GlobalAutoProfileStatus.SetCooldownUntil(w.cooldownUntil)
		return autoProfileAction{}, "cooldown"
	}

	w.pruneApplyHistory(now)
	if len(w.applyHistory) >= autoProfileMaxRestartsHr {
		return autoProfileAction{}, "budget"
	}

	summary := logsmetrics.GlobalSaturationHistory.Summary()
	current := getAutoProfileRuntimeValues(w.agent.config)
	limits := getAutoProfileLimits()
	action := decideAutoProfileAction(summary, current, limits)
	if action.name == "no_change" {
		return autoProfileAction{}, "no_change"
	}

	return action, ""
}

func (w *autoProfileWatchdog) apply(action autoProfileAction, now time.Time) error {
	cfg, ok := w.agent.config.(pkgconfigmodel.Config)
	if !ok {
		logsmetrics.TlmAutoProfileApply.Inc("failure", "config_not_writable")
		logsmetrics.GlobalAutoProfileStatus.RecordApply("failure", "config_not_writable", action.changes)
		return errors.New("auto profile requires writable config")
	}

	for k, v := range action.changes {
		cfg.Set(k, v, pkgconfigmodel.SourceAgentRuntime)
	}

	// Count attempts to protect against repeated restart churn, including failures.
	w.applyHistory = append(w.applyHistory, now)
	w.pruneApplyHistory(now)

	w.cooldownUntil = now.Add(autoProfileCooldown)
	logsmetrics.GlobalAutoProfileStatus.SetCooldownUntil(w.cooldownUntil)

	err := w.agent.restartWithCurrentTransport(context.Background())
	if err != nil {
		logsmetrics.TlmAutoProfileApply.Inc("failure", action.reason)
		logsmetrics.GlobalAutoProfileStatus.RecordApply("failure", action.reason, action.changes)
		return err
	}

	logsmetrics.TlmAutoProfileApply.Inc("success", action.reason)
	logsmetrics.GlobalAutoProfileStatus.RecordApply("success", action.reason, action.changes)
	return nil
}

func (w *autoProfileWatchdog) pruneApplyHistory(now time.Time) {
	cutoff := now.Add(-1 * time.Hour)
	n := 0
	for _, ts := range w.applyHistory {
		if ts.After(cutoff) {
			w.applyHistory[n] = ts
			n++
		}
	}
	w.applyHistory = w.applyHistory[:n]
}

func getAutoProfileRuntimeValues(cfg pkgconfigmodel.Reader) autoProfileRuntimeValues {
	return autoProfileRuntimeValues{
		pipelines:              cfg.GetInt("logs_config.pipelines"),
		batchMaxConcurrentSend: cfg.GetInt("logs_config.batch_max_concurrent_send"),
		useCompression:         cfg.GetBool("logs_config.use_compression"),
		compressionKind:        cfg.GetString("logs_config.compression_kind"),
		zstdCompressionLevel:   cfg.GetInt("logs_config.zstd_compression_level"),
		gzipCompressionLevel:   cfg.GetInt("logs_config.compression_level"),
	}
}

func getAutoProfileLimits() autoProfileLimits {
	maxPipelines := runtime.GOMAXPROCS(0)
	if maxPipelines < 1 {
		maxPipelines = 1
	}

	return autoProfileLimits{
		baselinePipelines:   min(4, maxPipelines),
		maxPipelines:        maxPipelines,
		baselineConcurrency: autoDefaultBatchMaxConcurrentSend,
	}
}

func compressionNormalized(v autoProfileRuntimeValues) bool {
	return v.useCompression &&
		v.compressionKind == logsconfig.ZstdCompressionKind &&
		v.zstdCompressionLevel == autoDefaultZstdCompressionLevel &&
		v.gzipCompressionLevel == autoDefaultGzipCompressionLevel
}

func normalizeCompressionChanges() map[string]interface{} {
	return map[string]interface{}{
		"logs_config.use_compression":        true,
		"logs_config.compression_kind":       logsconfig.ZstdCompressionKind,
		"logs_config.zstd_compression_level": autoDefaultZstdCompressionLevel,
		"logs_config.compression_level":      autoDefaultGzipCompressionLevel,
	}
}

func nextConcurrencyUp(current int) int {
	ladder := []int{0, 5, 10, 20}
	for _, v := range ladder {
		if v > current {
			return v
		}
	}
	return current
}

func nextConcurrencyDown(current int) int {
	ladder := []int{0, 5, 10, 20}
	for i := len(ladder) - 1; i >= 0; i-- {
		if ladder[i] < current {
			return ladder[i]
		}
	}
	return current
}

func decideAutoProfileAction(summary logsmetrics.SaturationSummary, current autoProfileRuntimeValues, limits autoProfileLimits) autoProfileAction {
	isStrategySaturated := summary.MaxFill5m[logsmetrics.StrategyTlmName] >= autoSaturationHighThreshold
	isSenderSaturated := summary.MaxFill5m[logsmetrics.SenderTlmName] >= autoSaturationHighThreshold
	isProcessorSaturated := summary.MaxFill5m[logsmetrics.ProcessorTlmName] >= autoSaturationHighThreshold

	recovered := summary.MaxFill5m[logsmetrics.StrategyTlmName] <= autoSaturationLowThreshold &&
		summary.MaxFill5m[logsmetrics.SenderTlmName] <= autoSaturationLowThreshold &&
		summary.MaxFill5m[logsmetrics.ProcessorTlmName] <= autoSaturationLowThreshold

	switch {
	case isStrategySaturated:
		if !compressionNormalized(current) {
			return autoProfileAction{
				name:    "normalize_compression",
				reason:  autoReasonStrategy,
				changes: normalizeCompressionChanges(),
			}
		}
		if current.pipelines < limits.maxPipelines {
			return autoProfileAction{
				name:   "increase_pipelines",
				reason: autoReasonStrategy,
				changes: map[string]interface{}{
					"logs_config.pipelines": current.pipelines + 1,
				},
			}
		}
	case isSenderSaturated:
		nextConcurrency := nextConcurrencyUp(current.batchMaxConcurrentSend)
		if nextConcurrency > current.batchMaxConcurrentSend {
			return autoProfileAction{
				name:   "increase_concurrency",
				reason: autoReasonSender,
				changes: map[string]interface{}{
					"logs_config.batch_max_concurrent_send": nextConcurrency,
				},
			}
		}
		if current.pipelines < limits.maxPipelines {
			return autoProfileAction{
				name:   "increase_pipelines",
				reason: autoReasonSender,
				changes: map[string]interface{}{
					"logs_config.pipelines": current.pipelines + 1,
				},
			}
		}
	case isProcessorSaturated:
		if current.pipelines < limits.maxPipelines {
			return autoProfileAction{
				name:   "increase_pipelines",
				reason: autoReasonProcess,
				changes: map[string]interface{}{
					"logs_config.pipelines": current.pipelines + 1,
				},
			}
		}
	case recovered:
		if current.pipelines > limits.baselinePipelines {
			return autoProfileAction{
				name:   "decrease_pipelines",
				reason: autoReasonRecovered,
				changes: map[string]interface{}{
					"logs_config.pipelines": current.pipelines - 1,
				},
			}
		}
		nextConcurrency := nextConcurrencyDown(current.batchMaxConcurrentSend)
		if nextConcurrency < current.batchMaxConcurrentSend && nextConcurrency >= limits.baselineConcurrency {
			return autoProfileAction{
				name:   "decrease_concurrency",
				reason: autoReasonRecovered,
				changes: map[string]interface{}{
					"logs_config.batch_max_concurrent_send": nextConcurrency,
				},
			}
		}
		if !compressionNormalized(current) {
			return autoProfileAction{
				name:    "normalize_compression",
				reason:  autoReasonRecovered,
				changes: normalizeCompressionChanges(),
			}
		}
	default:
		return autoProfileAction{name: "no_change", reason: autoReasonNoBottleneck}
	}

	return autoProfileAction{name: "no_change", reason: autoReasonNoBottleneck}
}

func (a *logAgent) startAutoProfileWatchdog(trigger string) {
	a.autoProfileMu.Lock()
	defer a.autoProfileMu.Unlock()
	if a.autoProfileWatchdog != nil {
		return
	}

	w := newAutoProfileWatchdog(a)
	a.autoProfileWatchdog = w
	w.start()

	logsmetrics.TlmAutoProfileEnabled.Set(1)
	logsmetrics.GlobalAutoProfileStatus.SetEnabled(true)
	logsmetrics.GlobalAutoProfileStatus.RecordDecision("started", trigger)
	a.log.Infof("Started auto profile watchdog (%s)", trigger)
}

func (a *logAgent) stopAutoProfileWatchdog(trigger string) {
	a.autoProfileMu.Lock()
	w := a.autoProfileWatchdog
	a.autoProfileWatchdog = nil
	a.autoProfileMu.Unlock()

	if w != nil {
		w.stop()
	}

	logsmetrics.TlmAutoProfileEnabled.Set(0)
	logsmetrics.GlobalAutoProfileStatus.SetEnabled(false)
	logsmetrics.GlobalAutoProfileStatus.SetCooldownUntil(time.Time{})
	logsmetrics.GlobalAutoProfileStatus.RecordDecision("stopped", trigger)
}

func (a *logAgent) clearAutoProfileRuntimeOverrides() (bool, error) {
	cfg, ok := a.config.(pkgconfigmodel.Config)
	if !ok {
		return false, errors.New("auto profile requires writable config")
	}

	cleared := false
	for _, key := range autoProfileControlledKeys {
		if cfg.GetSource(key) == pkgconfigmodel.SourceAgentRuntime {
			cfg.UnsetForSource(key, pkgconfigmodel.SourceAgentRuntime)
			cleared = true
		}
	}
	return cleared, nil
}

func (a *logAgent) disableAutoProfileAndRestore(trigger string) {
	a.stopAutoProfileWatchdog(trigger)

	cleared, err := a.clearAutoProfileRuntimeOverrides()
	if err != nil {
		logsmetrics.TlmAutoProfileApply.Inc("failure", "clear_overrides")
		logsmetrics.GlobalAutoProfileStatus.RecordApply("failure", "clear_overrides", map[string]interface{}{})
		a.log.Errorf("Failed to clear auto profile overrides: %v", err)
		return
	}
	if cleared {
		logsmetrics.GlobalAutoProfileStatus.RecordDecision("restore_defaults", "auto_disabled")
	} else {
		logsmetrics.GlobalAutoProfileStatus.RecordDecision("restart", "auto_disabled")
	}

	if err := a.restartWithCurrentTransport(context.Background()); err != nil {
		logsmetrics.TlmAutoProfileApply.Inc("failure", "auto_disabled")
		logsmetrics.GlobalAutoProfileStatus.RecordApply("failure", "auto_disabled", map[string]interface{}{})
		a.log.Warnf("Failed to restart while disabling auto profile: %v", err)
		return
	}

	logsmetrics.TlmAutoProfileApply.Inc("success", "auto_disabled")
	logsmetrics.GlobalAutoProfileStatus.RecordApply("success", "auto_disabled", map[string]interface{}{})
}

func (a *logAgent) registerAutoProfileModeCallback(cfg configComponent.Component) {
	cfg.OnUpdate(func(setting string, _ pkgconfigmodel.Source, _, _ any, _ uint64) {
		if setting != autoProfileConfigKey {
			return
		}
		if a.started.Load() != logsstatus.StatusRunning {
			return
		}

		if logsconfig.IsAutoProfileEnabled(a.config) {
			a.startAutoProfileWatchdog("config_update")
			return
		}
		go a.disableAutoProfileAndRestore("config_update")
	})
}

func (a *logAgent) onStartAutoProfileMode() error {
	if logsconfig.IsAutoProfileEnabled(a.config) {
		a.startAutoProfileWatchdog("startup")
		return nil
	}

	logsmetrics.TlmAutoProfileEnabled.Set(0)
	logsmetrics.GlobalAutoProfileStatus.SetEnabled(false)
	logsmetrics.GlobalAutoProfileStatus.SetCooldownUntil(time.Time{})
	return nil
}

func (a *logAgent) onStopAutoProfileMode() {
	a.stopAutoProfileWatchdog("shutdown")
}
