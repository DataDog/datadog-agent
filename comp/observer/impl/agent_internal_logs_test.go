// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"hash/fnv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/pattern"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/require"
)

func TestAgentInternalLogsFlowIntoObserver(t *testing.T) {
	// Ensure util/log is initialized so log calls actually emit (otherwise they buffer pre-init).
	pkglog.SetupLogger(pkglog.Disabled(), "info")

	provides := NewComponent(Requires{})
	obs, ok := provides.Comp.(*observerImpl)
	require.True(t, ok)
	t.Cleanup(func() { pkglog.SetLogObserver(nil) })

	msg := "agent internal hello"
	pkglog.Info(msg)

	// Agent logs are forwarded as structured JSON: {"msg":"..."}.
	payload := []byte(`{"msg":"agent internal hello"}`)
	sig := pattern.Signature(payload, 4096)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	metricName := "log.pattern." + toHex64(h.Sum64()) + ".count"
	tags := []string{"source:datadog-agent", "level:info"}

	// Poll briefly since observer processes asynchronously.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := obs.storage.GetSeries("agent-internal-logs", metricName, tags, AggregateSum); s != nil && len(s.Points) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected series not found for agent internal logs: %s", metricName)
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
