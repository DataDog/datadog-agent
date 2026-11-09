// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// captureObserverHandle records every ObserveLog call for inspection in tests.
type captureObserverHandle struct {
	logs []observer.LogView
}

func (h *captureObserverHandle) ObserveMetric(_ observer.MetricView, _ uint64) {}
func (h *captureObserverHandle) ObserveLog(v observer.LogView) {
	h.logs = append(h.logs, v)
}

// drainMessages feeds msgs into p.outputChan, closes it, then waits for
// drainOutputChan to finish — exercising the real production code path.
func drainMessages(p *observerPipeline, msgs []*message.Message) {
	go p.drainOutputChan()
	for _, m := range msgs {
		p.outputChan <- m
	}
	close(p.outputChan)
	<-p.drainDone
}

func newLogMessage(t *testing.T, content, logSource, identifier string, tags []string) *message.Message {
	t.Helper()
	src := sources.NewLogSource(identifier, &logsconfig.LogsConfig{
		Source:     logSource,
		Identifier: identifier,
	})
	origin := message.NewOrigin(src)
	origin.SetSource(logSource)
	origin.SetTags(tags)
	return message.NewMessage([]byte(content), origin, "info", time.Now().UnixNano())
}

func TestPipeline_ForwardsMessagesAndPreservesSourceMetadata(t *testing.T) {
	handle := &captureObserverHandle{}
	p := &observerPipeline{
		outputChan:     make(chan *message.Message, 10),
		drainDone:      make(chan struct{}),
		observerHandle: handle,
	}

	containerMsg := newLogMessage(t, "hello", "containerd", "abc", []string{"env:prod"})
	customMsg := newLogMessage(t, "nginx log", "nginx", "xyz", []string{"service:web"})
	drainMessages(p, []*message.Message{containerMsg, customMsg})

	require.Len(t, handle.logs, 2)
	assert.Equal(t, "containerd", handle.logs[0].GetSource())
	assert.Equal(t, "hello", handle.logs[0].GetContent())
	assert.Equal(t, []string{"env:prod"}, handle.logs[0].Tags())
	assert.Equal(t, "nginx", handle.logs[1].GetSource())
	assert.Equal(t, "nginx log", handle.logs[1].GetContent())
	assert.Equal(t, []string{"service:web"}, handle.logs[1].Tags())
}
