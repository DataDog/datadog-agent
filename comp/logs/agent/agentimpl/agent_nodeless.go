// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubelet

package agentimpl

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
)

const (
	nodelessCheckMaxAttempts  = 20
	nodelessCheckInitialDelay = 2 * time.Second
	nodelessCheckMaxDelay     = 30 * time.Second
)

// startNodelessDetection runs a background goroutine that detects whether this
// agent is running on a nodeless node. If detected after startup it reconfigures
// the pipeline to disable gRPC and strip routing headers. Async so it never
// blocks agent startup regardless of cluster agent availability.
func (a *logAgent) startNodelessDetection() {
	go func() {
		delay := nodelessCheckInitialDelay
		for attempt := 1; attempt <= nodelessCheckMaxAttempts; attempt++ {
			nodeInfo, err := hostinfo.NewNodeInfo()
			if err != nil {
				a.log.Debugf("logs-agent: nodeless check attempt %d/%d: NewNodeInfo: %v", attempt, nodelessCheckMaxAttempts, err)
				time.Sleep(delay)
				delay = min(delay*2, nodelessCheckMaxDelay)
				continue
			}
			labels, err := nodeInfo.GetNodeLabels(context.Background())
			if err != nil {
				a.log.Debugf("logs-agent: nodeless check attempt %d/%d: GetNodeLabels: %v", attempt, nodelessCheckMaxAttempts, err)
				time.Sleep(delay)
				delay = min(delay*2, nodelessCheckMaxDelay)
				continue
			}
			if labels["class"] == "nodeless" {
				a.log.Infof("logs-agent: nodeless node detected after %d attempt(s), reconfiguring pipeline", attempt)
				a.reconfigureForNodeless()
			} else {
				a.log.Infof("logs-agent: nodeless check complete after %d attempt(s): not nodeless (class=%q)", attempt, labels["class"])
			}
			return
		}
		a.log.Warnf("logs-agent: could not determine nodeless status after %d attempts, keeping current config", nodelessCheckMaxAttempts)
	}()
}

// reconfigureForNodeless builds a copy of the current endpoints with all gRPC
// disabled and routing headers stripped, then restarts the pipeline via the
// standard restart() path (which handles rollback on failure).
func (a *logAgent) reconfigureForNodeless() {
	// Copy the current endpoints so restart() can use them for rollback.
	current := *a.endpoints
	endpoints := &current
	// Deep-copy slices and maps so we don't mutate shared state.
	endpoints.Endpoints = make([]config.Endpoint, len(a.endpoints.Endpoints))
	copy(endpoints.Endpoints, a.endpoints.Endpoints)
	mainHeaders := make(map[string]string, len(a.endpoints.Main.ExtraHTTPHeaders))
	for k, v := range a.endpoints.Main.ExtraHTTPHeaders {
		mainHeaders[k] = v
	}
	endpoints.Main.ExtraHTTPHeaders = mainHeaders

	changed := false
	if endpoints.UseGRPC {
		endpoints.UseGRPC = false
		endpoints.UseHTTP = true
		changed = true
	}
	for i := range endpoints.Endpoints {
		if endpoints.Endpoints[i].UseGRPC {
			endpoints.Endpoints[i].UseGRPC = false
			changed = true
		}
	}
	endpoints.Nodeless = true

	if !changed && !a.endpoints.Nodeless {
		// gRPC was already disabled but Nodeless flag wasn't set — still restart to strip headers.
	} else if !changed {
		a.log.Info("logs-agent: nodeless reconfigure: already configured for nodeless")
		return
	}

	delete(endpoints.Main.ExtraHTTPHeaders, "x-dd-logs-routing")
	for i := range endpoints.Endpoints {
		delete(endpoints.Endpoints[i].ExtraHTTPHeaders, "x-dd-logs-routing")
	}

	if err := a.restart(context.Background(), endpoints); err != nil {
		a.log.Errorf("logs-agent: nodeless reconfigure failed: %v", err)
	}
}
