// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package instrumentationstatus reports node-side check configuration and run
// results for checks originating from DatadogInstrumentation resources.
package instrumentationstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const (
	reportBufferSize = 128
	maxErrorLength   = 1024
)

// Client posts reports to the leader Cluster Agent.
type Client interface {
	PostInstrumentationCheckStatus(context.Context, cctypes.InstrumentationCheckStatusRequest) error
}

type checkOrigin struct {
	origin        integration.InstrumentationConfigOrigin
	instanceIndex int
	checkName     string
}

type reporter struct {
	mu        sync.RWMutex
	client    Client
	checks    map[checkid.ID]checkOrigin
	lastSent  map[string]string
	reports   chan cctypes.InstrumentationCheckStatusReport
	startOnce sync.Once
}

var globalReporter = &reporter{
	checks:   make(map[checkid.ID]checkOrigin),
	lastSent: make(map[string]string),
	reports:  make(chan cctypes.InstrumentationCheckStatusReport, reportBufferSize),
}

// SetClient configures delivery to the Cluster Agent and starts the async
// reporter. Calling it again replaces the client used for future reports.
func SetClient(client Client) {
	globalReporter.mu.Lock()
	globalReporter.client = client
	globalReporter.mu.Unlock()
	globalReporter.startOnce.Do(func() { go globalReporter.run() })
}

// RegisterCheck associates a loaded check with its DDI origin.
func RegisterCheck(id checkid.ID, origin *integration.InstrumentationConfigOrigin, instanceIndex int, checkName string) {
	if origin == nil {
		return
	}
	globalReporter.mu.Lock()
	globalReporter.checks[id] = checkOrigin{origin: *origin, instanceIndex: instanceIndex, checkName: checkName}
	globalReporter.mu.Unlock()
}

// UnregisterCheck forgets a check after it is unscheduled.
func UnregisterCheck(id checkid.ID) {
	globalReporter.mu.Lock()
	delete(globalReporter.checks, id)
	globalReporter.mu.Unlock()
}

// ReportConfigureFailure reports that no loader could configure an instance.
func ReportConfigureFailure(origin *integration.InstrumentationConfigOrigin, instanceIndex int, checkName string, err error) {
	if origin == nil || err == nil {
		return
	}
	globalReporter.enqueue(newReport(*origin, instanceIndex, checkName, "", cctypes.InstrumentationCheckStatusConfigure, cctypes.InstrumentationCheckStatusFailed, err))
}

// ReportRunResult records a running check's latest result. It is called after
// every run, but the async reporter only sends a POST when the state or error
// differs from the last successfully delivered report.
func ReportRunResult(id checkid.ID, err error) {
	globalReporter.mu.RLock()
	registered, found := globalReporter.checks[id]
	globalReporter.mu.RUnlock()
	if !found {
		return
	}
	state := cctypes.InstrumentationCheckStatusRunning
	if err != nil {
		state = cctypes.InstrumentationCheckStatusFailed
	}
	globalReporter.enqueue(newReport(registered.origin, registered.instanceIndex, registered.checkName, string(id), cctypes.InstrumentationCheckStatusRun, state, err))
}

func newReport(origin integration.InstrumentationConfigOrigin, instanceIndex int, checkName, id string, phase cctypes.InstrumentationCheckStatusPhase, state cctypes.InstrumentationCheckStatusState, err error) cctypes.InstrumentationCheckStatusReport {
	report := cctypes.InstrumentationCheckStatusReport{
		Namespace: origin.Namespace, Name: origin.Name, UID: origin.UID, Generation: origin.Generation,
		CheckIndex: origin.CheckIndex, InstanceIndex: instanceIndex, CheckName: checkName, CheckID: id,
		NodeName: pkgconfigsetup.Datadog().GetString("kubernetes_kubelet_nodename"), Phase: phase, State: state,
	}
	if err != nil {
		report.Error, _ = scrubber.ScrubString(err.Error())
		if report.Error == "" {
			report.Error = scrubber.ScrubLine(err.Error())
		}
		if len(report.Error) > maxErrorLength {
			report.Error = report.Error[:maxErrorLength]
		}
	}
	return report
}

func (r *reporter) enqueue(report cctypes.InstrumentationCheckStatusReport) {
	select {
	case r.reports <- report:
	default:
		log.Warnf("Dropping DatadogInstrumentation check status for %s/%s: reporter queue is full", report.Namespace, report.Name)
	}
}

func (r *reporter) run() {
	for report := range r.reports {
		key := fmt.Sprintf("%s/%s/%s/%d/%d/%d/%s/%s", report.Namespace, report.Name, report.UID, report.Generation, report.CheckIndex, report.InstanceIndex, report.NodeName, report.Phase)
		signature := string(report.State) + "\x00" + report.Error

		r.mu.RLock()
		client := r.client
		unchanged := r.lastSent[key] == signature
		r.mu.RUnlock()
		if client == nil || unchanged {
			continue
		}

		var err error
		for attempt := 0; attempt < 3; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = client.PostInstrumentationCheckStatus(ctx, cctypes.InstrumentationCheckStatusRequest{Reports: []cctypes.InstrumentationCheckStatusReport{report}})
			cancel()
			if err == nil {
				break
			}
			if attempt < 2 {
				time.Sleep(time.Second)
			}
		}
		if err != nil {
			log.Warnf("Unable to report DatadogInstrumentation check status for %s/%s: %v", report.Namespace, report.Name, err)
			continue
		}
		r.mu.Lock()
		r.lastSent[key] = signature
		r.mu.Unlock()
	}
}
