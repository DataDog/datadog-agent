// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const agentLogSource = "datadog-agent"

// installAgentLogTap registers a pkg/util/log observer that forwards agent-internal
// log lines into the observer pipeline. Logs below minSeverity are dropped before
// any rate-limiting. The three max rates are in logs/second over a 10-second
// window: maxRateHigh (warn/error/critical), maxRateMedium (info), maxRateLow
// (trace/debug). -1 means unlimited; 0 drops all.
// onDropped is called with the priority bucket name ("high", "medium", "low")
// when a log is dropped by the rate limiter. It is NOT called for min_severity
// drops. It may be nil.
// rules is applied after the severity gate but before the rate gate, so excluded
// messages do not consume rate budget. A nil rules allows all.
func installAgentLogTap(handle observerdef.Handle, minSeverity string, maxRateHigh, maxRateMedium, maxRateLow float64, onDropped func(priority string), rules *logsfilter.Rules) {
	baseTags := []string{"source:" + agentLogSource}
	minBucket := logsfilter.MinBucketForSeverity(minSeverity)

	var highW, mediumW, lowW logsfilter.RateWindow
	// chargeRate consumes one slot from the appropriate rate window and returns
	// (true, "") if allowed or (false, tier) if rate-limited.
	chargeRate := func(level pkglog.LogLevel) (bool, string) {
		tier := logsfilter.RateTierForBucket(logsfilter.BucketForStatus(strings.ToLower(level.String())))
		var allowed bool
		switch tier {
		case "high":
			allowed = highW.Allow(maxRateHigh)
		case "medium":
			allowed = mediumW.Allow(maxRateMedium)
		default:
			allowed = lowW.Allow(maxRateLow)
		}
		if allowed {
			return true, ""
		}
		return false, tier
	}

	pkglog.SetLogObserver(func(level pkglog.LogLevel, message string) {
		// 1. Severity gate: cheap, no side effects.
		bucket := logsfilter.BucketForStatus(strings.ToLower(level.String()))
		if bucket < minBucket {
			return
		}
		// 2. Build tags and apply processing rules before consuming rate budget.
		tags := make([]string, 0, 3)
		tags = append(tags, baseTags...)
		if name := pkglog.GetLoggerName(); name != "" {
			tags = append(tags, "component:"+name)
		}
		tags = append(tags, "level:"+strings.ToLower(level.String()))
		if rules.NeedsSortedTags() {
			slices.Sort(tags)
		}
		if !rules.IsAllowed(agentLogSource, tags) {
			return
		}
		// 3. Rate limiting: only charge budget for messages that pass the rule filter.
		if forward, droppedPriority := chargeRate(level); !forward {
			if droppedPriority != "" && onDropped != nil {
				onDropped(droppedPriority)
			}
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"msg": message,
		})
		handle.ObserveLog(&agentLogView{
			content:     string(payload),
			status:      strings.ToLower(level.String()),
			tags:        tags,
			hostname:    "",
			timestampMs: time.Now().UnixMilli(),
		})
	})
}

// agentLogView is a minimal observerdef.LogView implementation for agent-internal logs.
type agentLogView struct {
	content     string
	status      string
	tags        []string
	hostname    string
	timestampMs int64
}

func (v *agentLogView) GetContent() string           { return v.content }
func (v *agentLogView) GetStatus() string            { return v.status }
func (v *agentLogView) Tags() []string               { return v.tags }
func (v *agentLogView) GetHostname() string          { return v.hostname }
func (v *agentLogView) GetTimestampUnixMilli() int64 { return v.timestampMs }
