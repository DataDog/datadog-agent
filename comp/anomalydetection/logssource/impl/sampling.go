// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// sourceRateLimits holds the min_severity gate and per-priority max rates for
// one log source. MaxRate values are in logs/second; -1 means unlimited, 0 drops all.
type sourceRateLimits struct {
	minSeverity   string
	maxRateHigh   float64 // logs/s for high-priority (warn and above); -1 = unlimited, 0 = drop all
	maxRateMedium float64 // logs/s for medium-priority (info); -1 = unlimited, 0 = drop all
	maxRateLow    float64 // logs/s for low-priority (trace/debug); -1 = unlimited, 0 = drop all
}

// logSampler applies per-source min_severity gating and rate limiting to log
// messages before they are forwarded to the observer. A fixed 10-second window
// is used per (source, priority) pair.
type logSampler struct {
	kubelet    sourceRateLimits
	containers sourceRateLimits

	kubeletHigh     logsfilter.RateWindow
	kubeletMedium   logsfilter.RateWindow
	kubeletLow      logsfilter.RateWindow
	containerHigh   logsfilter.RateWindow
	containerMedium logsfilter.RateWindow
	containerLow    logsfilter.RateWindow

	// onDropped is called with (source, priority) when a log is dropped by the
	// rate limiter. It does NOT fire for min_severity drops. May be nil.
	onDropped func(source, priority string)
}

// newLogSampler constructs a logSampler. onDropped is called on every
// rate-limit drop and may be nil.
func newLogSampler(kubelet, containers sourceRateLimits, onDropped func(source, priority string)) *logSampler {
	return &logSampler{
		kubelet:    kubelet,
		containers: containers,
		onDropped:  onDropped,
	}
}

// newLogSamplerFromConfig reads anomaly_detection.logs.kubelet.* and
// anomaly_detection.logs.containers.* from the agent config and returns a
// logSampler ready for use. onDropped is wired to the observer component so
// rate-limit drops are recorded in the shared sampler_dropped counter.
func newLogSamplerFromConfig(cfg pkgconfigmodel.Reader, onDropped func(source, priority string)) *logSampler {
	kubelet := sourceRateLimits{
		minSeverity:   cfg.GetString("anomaly_detection.logs.kubelet.min_severity"),
		maxRateHigh:   cfg.GetFloat64("anomaly_detection.logs.kubelet.max_rate_high_priority"),
		maxRateMedium: cfg.GetFloat64("anomaly_detection.logs.kubelet.max_rate_medium_priority"),
		maxRateLow:    cfg.GetFloat64("anomaly_detection.logs.kubelet.max_rate_low_priority"),
	}
	containers := sourceRateLimits{
		minSeverity:   cfg.GetString("anomaly_detection.logs.containers.min_severity"),
		maxRateHigh:   cfg.GetFloat64("anomaly_detection.logs.containers.max_rate_high_priority"),
		maxRateMedium: cfg.GetFloat64("anomaly_detection.logs.containers.max_rate_medium_priority"),
		maxRateLow:    cfg.GetFloat64("anomaly_detection.logs.containers.max_rate_low_priority"),
	}
	return newLogSampler(kubelet, containers, onDropped)
}

// ShouldForward returns true if the message should be forwarded to the observer.
// Source is detected via the "source:kubelet" tag injected by kubelet_source.go;
// everything else is treated as a container log.
func (s *logSampler) ShouldForward(msg *message.Message) bool {
	if isKubeletMessage(msg) {
		return s.shouldForwardSource(msg.GetStatus(), s.kubelet, "kubelet",
			&s.kubeletHigh, &s.kubeletMedium, &s.kubeletLow)
	}
	return s.shouldForwardSource(msg.GetStatus(), s.containers, "containers",
		&s.containerHigh, &s.containerMedium, &s.containerLow)
}

func (s *logSampler) shouldForwardSource(status string, limits sourceRateLimits, source string, highW, mediumW, lowW *logsfilter.RateWindow) bool {
	bucket := logsfilter.BucketForStatus(status)
	if bucket < logsfilter.MinBucketForSeverity(limits.minSeverity) {
		return false
	}
	tier := logsfilter.RateTierForBucket(bucket)
	var allowed bool
	switch tier {
	case "high":
		allowed = highW.Allow(limits.maxRateHigh)
	case "medium":
		allowed = mediumW.Allow(limits.maxRateMedium)
	default:
		allowed = lowW.Allow(limits.maxRateLow)
	}
	if allowed {
		return true
	}
	if s.onDropped != nil {
		s.onDropped(source, tier)
	}
	return false
}

// isKubeletMessage reports whether msg originates from the kubelet journald source.
func isKubeletMessage(msg *message.Message) bool {
	for _, t := range msg.Tags() {
		if t == "source:kubelet" {
			return true
		}
	}
	return false
}
