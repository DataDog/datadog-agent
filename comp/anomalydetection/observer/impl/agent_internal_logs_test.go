// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"hash/fnv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestAgentInternalLogsFlowIntoObserver(t *testing.T) {
	// Ensure util/log is initialized so log calls actually emit (otherwise they buffer pre-init).
	pkglog.SetupLogger(pkglog.Disabled(), "info")
	pkglog.SetLoggerName("CORE")

	// Enable analysis pipeline so GetHandle returns a real handle (not noop).
	cfg := configmock.New(t)
	cfg.Set("observer.analysis.enabled", true, model.SourceAgentRuntime)
	cfg.SetWithoutSource("observer.capture_agent_internal_logs.enabled", true)
	cfg.SetWithoutSource("observer.capture_agent_internal_logs.sample_rate_info", 1.0)
	cfg.SetWithoutSource("observer.capture_agent_internal_logs.sample_rate_debug", 1.0)
	cfg.SetWithoutSource("observer.capture_agent_internal_logs.sample_rate_trace", 1.0)

	provides := NewComponent(Requires{
		Telemetry: telemetry.New(t),
		Config:    cfg,
		Reporters: []reporterdef.Reporter{&noopTestReporter{}},
	})
	obs, ok := provides.Comp.(*observerImpl)
	require.True(t, ok)
	t.Cleanup(func() { pkglog.SetLogObserver(nil) })

	msg := "agent internal hello"
	pkglog.Info(msg)

	// Agent logs are forwarded as structured JSON: {"msg":"..."}.
	payload := []byte(`{"msg":"agent internal hello"}`)
	sig := logSignature(payload, 4096)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	metricName := "log.pattern." + toHex64(h.Sum64()) + ".count"
	tags := []string{"component:core", "level:info", "observer_source:agent-internal-logs", "source:datadog-agent"}

	// Poll briefly since observer processes asynchronously.
	// Namespace is the extractor component name (log_metrics_extractor).
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		s := obs.engine.Storage().GetSeries("log_metrics_extractor", metricName, tags, AggregateSum)
		require.NotNil(collect, s)
		require.Greater(collect, len(s.Points), 0)
	}, time.Second*5, time.Millisecond*10)
}

func toHex64(v uint64) string {
	const hextable = "0123456789abcdef"
	var out [16]byte
	for i := 15; i >= 0; i-- {
		out[i] = hextable[v&0xF]
		v >>= 4
	}
	// Mirror fmt.Sprintf("%x", ...) (no leading zeros trimmed? actually %x trims; we keep full width here but it won't match)
	// Trim leading zeros for parity with production metric naming (fmt %x).
	i := 0
	for i < 15 && out[i] == '0' {
		i++
	}
	return string(out[i:])
}
