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

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// captureObserverHandle records every ObserveLog call for inspection in tests.
type captureObserverHandle struct {
	logs []observer.LogView
}

func (h *captureObserverHandle) ObserveMetric(_ observer.MetricView)                   {}
func (h *captureObserverHandle) ObserveMetricAndReportDrop(_ observer.MetricView) bool { return false }
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

func TestPipeline_NilRulesForwardsAll(t *testing.T) {
	handle := &captureObserverHandle{}
	p := &observerPipeline{
		outputChan:     make(chan *message.Message, 10),
		drainDone:      make(chan struct{}),
		observerHandle: handle,
		rules:          nil,
	}

	msg := newLogMessage(t, "hello", "containerd", "abc", nil)
	drainMessages(p, []*message.Message{msg})

	require.Len(t, handle.logs, 1)
}

func TestPipeline_TagRuleDropsMessage(t *testing.T) {
	rules, err := logsfilter.NewRules([]logsfilter.ProcessingRule{
		{Name: "drop_dev", Type: "exclude_at_match", Tags: []string{"env:dev"}},
	})
	require.NoError(t, err)

	handle := &captureObserverHandle{}
	p := &observerPipeline{
		outputChan:     make(chan *message.Message, 10),
		drainDone:      make(chan struct{}),
		observerHandle: handle,
		rules:          rules,
	}

	devMsg := newLogMessage(t, "dev log", "containerd", "abc", []string{"env:dev"})
	prodMsg := newLogMessage(t, "prod log", "containerd", "xyz", []string{"env:prod"})
	drainMessages(p, []*message.Message{devMsg, prodMsg})

	require.Len(t, handle.logs, 1, "only the prod message must be forwarded")
	assert.Equal(t, "prod log", string(handle.logs[0].GetContent()))
}

func TestPipeline_TagRuleDropsMessageSourceKeepsRunning(t *testing.T) {
	rules, err := logsfilter.NewRules([]logsfilter.ProcessingRule{
		{Name: "drop_dev", Type: "exclude_at_match", Tags: []string{"env:dev"}},
	})
	require.NoError(t, err)

	handle := &captureObserverHandle{}
	p := &observerPipeline{
		outputChan:     make(chan *message.Message, 10),
		drainDone:      make(chan struct{}),
		observerHandle: handle,
		rules:          rules,
	}

	// Two messages from the same container: first excluded, second included.
	// Both use the same source identifier to confirm no source-level side-effect.
	devMsg := newLogMessage(t, "dev log", "containerd", "abc", []string{"env:dev"})
	prodMsg := newLogMessage(t, "prod log", "containerd", "abc", []string{"env:prod"})
	drainMessages(p, []*message.Message{devMsg, prodMsg})

	require.Len(t, handle.logs, 1, "excluded message dropped; subsequent message from same source forwarded")
	assert.Equal(t, "prod log", string(handle.logs[0].GetContent()))
}

func TestPipeline_SourceRuleDropsMessage(t *testing.T) {
	rules, err := logsfilter.NewRules([]logsfilter.ProcessingRule{
		{Name: "drop_kubelet", Type: "exclude_at_match", Source: "kubelet"},
	})
	require.NoError(t, err)

	handle := &captureObserverHandle{}
	p := &observerPipeline{
		outputChan:     make(chan *message.Message, 10),
		drainDone:      make(chan struct{}),
		observerHandle: handle,
		rules:          rules,
	}

	kubeletMsg := newLogMessage(t, "kubelet log", "kubelet", "kubelet", nil)
	containerMsg := newLogMessage(t, "container log", "containerd", "abc", nil)
	drainMessages(p, []*message.Message{kubeletMsg, containerMsg})

	require.Len(t, handle.logs, 1, "only the container message must be forwarded")
	assert.Equal(t, "container log", string(handle.logs[0].GetContent()))
}

func TestPipeline_IncludeBeforeExcludeAllowsMatching(t *testing.T) {
	rules, err := logsfilter.NewRules([]logsfilter.ProcessingRule{
		{Name: "keep_prod", Type: "include_at_match", Tags: []string{"env:prod"}},
		{Name: "drop_all", Type: "exclude_at_match"},
	})
	require.NoError(t, err)

	handle := &captureObserverHandle{}
	p := &observerPipeline{
		outputChan:     make(chan *message.Message, 10),
		drainDone:      make(chan struct{}),
		observerHandle: handle,
		rules:          rules,
	}

	prodMsg := newLogMessage(t, "prod log", "containerd", "abc", []string{"env:prod"})
	devMsg := newLogMessage(t, "dev log", "containerd", "xyz", []string{"env:dev"})
	drainMessages(p, []*message.Message{prodMsg, devMsg})

	require.Len(t, handle.logs, 1)
	assert.Equal(t, "prod log", string(handle.logs[0].GetContent()))
}
