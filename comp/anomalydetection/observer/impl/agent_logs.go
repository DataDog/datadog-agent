// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// installAgentLogTap registers a pkg/util/log observer that forwards agent-internal
// log lines into the observer pipeline.  WARN/ERROR/CRITICAL are always forwarded;
// INFO/DEBUG/TRACE are sampled at the provided rates (0.0–1.0).
func installAgentLogTap(handle observerdef.Handle, sampleInfo, sampleDebug, sampleTrace float64) {
	baseTags := []string{"source:datadog-agent"}

	var infoN, debugN, traceN uint64
	shouldSample := func(level pkglog.LogLevel) bool {
		switch level {
		case pkglog.WarnLvl, pkglog.ErrorLvl, pkglog.CriticalLvl:
			return true
		case pkglog.InfoLvl:
			return samplePass(sampleInfo, atomic.AddUint64(&infoN, 1))
		case pkglog.DebugLvl:
			return samplePass(sampleDebug, atomic.AddUint64(&debugN, 1))
		case pkglog.TraceLvl:
			return samplePass(sampleTrace, atomic.AddUint64(&traceN, 1))
		default:
			return samplePass(sampleInfo, atomic.AddUint64(&infoN, 1))
		}
	}

	pkglog.SetLogObserver(func(level pkglog.LogLevel, message string) {
		if !shouldSample(level) {
			return
		}
		tags := make([]string, 0, 3)
		tags = append(tags, baseTags...)
		if name := pkglog.GetLoggerName(); name != "" {
			tags = append(tags, "component:"+name)
		}
		tags = append(tags, "level:"+strings.ToLower(level.String()))
		payload, _ := json.Marshal(map[string]any{
			"msg": message,
		})
		handle.ObserveLog(&agentLogView{
			content:     payload,
			status:      strings.ToLower(level.String()),
			tags:        tags,
			hostname:    "",
			timestampMs: time.Now().UnixMilli(),
		})
	})
}

// samplePass returns true at approximately rate (0.0–1.0) of calls, using a
// deterministic modulo counter so the sampling distribution is even.
func samplePass(rate float64, n uint64) bool {
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	const denom = 1000
	threshold := uint64(rate * denom)
	if threshold == 0 {
		threshold = 1
	}
	return (n % denom) < threshold
}

// agentLogView is a minimal observerdef.LogView implementation for agent-internal logs.
type agentLogView struct {
	content     []byte
	status      string
	tags        []string
	hostname    string
	timestampMs int64
}

func (v *agentLogView) GetContent() []byte           { return v.content }
func (v *agentLogView) GetStatus() string            { return v.status }
func (v *agentLogView) Tags() []string               { return v.tags }
func (v *agentLogView) GetHostname() string          { return v.hostname }
func (v *agentLogView) GetTimestampUnixMilli() int64 { return v.timestampMs }
