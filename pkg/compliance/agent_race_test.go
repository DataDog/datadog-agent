// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"expvar"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// TestReportCheckEventsConcurrentStatusRead races reportCheckEvents() against
// concurrent status reads, mirroring the expvar/status endpoint. Run with -race.
func TestReportCheckEventsConcurrentStatusRead(t *testing.T) {
	const containerID = "abc123"

	wmetaMock := workloadmetaimpl.NewWorkloadMetaMock(workloadmetaimpl.Dependencies{
		Lc:     compdef.NewTestLifecycle(t),
		Log:    logmock.New(t),
		Config: config.NewMock(t),
		Params: workloadmeta.NewParams(),
	})
	wmetaMock.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image: workloadmeta.ContainerImage{
			ID:   "sha256:deadbeef",
			Name: "redis",
			Tag:  "latest",
		},
	})

	// LogReporter is built directly (same package) with a drained, buffered
	// channel instead of a real network pipeline, so ReportEvent never blocks.
	logChan := make(chan *message.Message, 100)
	go func() {
		for msg := range logChan {
			_ = msg
		}
	}()
	reporter := &LogReporter{
		hostname:  "test-host",
		logSource: sources.NewLogSource("test", &logsconfig.LogsConfig{Type: "test", Source: "test"}),
		logChan:   logChan,
		endpoints: &logsconfig.Endpoints{},
	}

	managedEnv := "test-managed-env"
	a := &Agent{
		wmeta: wmetaMock,
		opts:  AgentOptions{Reporter: reporter},
		statuses: map[string]*CheckStatus{
			"rule-1": {RuleID: "rule-1"},
		},
		k8sManaged: &managedEnv,
	}

	statusFn := expvar.Func(func() interface{} { return a.getChecksStatus() })

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = statusFn.String()
			}
		}
	})

	for i := 0; i < 2000; i++ {
		event := &CheckEvent{
			RuleID: "rule-1",
			Result: CheckPassed,
			Container: &CheckContainerMeta{
				ContainerID: containerID,
			},
		}
		a.reportCheckEvents(time.Minute, event)
	}

	close(stop)
	wg.Wait()
}
