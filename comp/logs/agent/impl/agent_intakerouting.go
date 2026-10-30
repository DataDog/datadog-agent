// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"context"

	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// intakeRoutingState is the intake routing registration of the logs agent's endpoints.
type intakeRoutingState struct {
	// base is the endpoints built from the configuration, before routing.
	base *config.Endpoints
	// reg routes the endpoints of base that qualify. It is nil when none does.
	reg intakerouting.Registration
	// indices maps each request of reg to its index in base.Endpoints.
	indices []int
	// done stops the goroutine watching reg.
	done chan struct{}
}

// close stops routing the endpoints of s.
func (s *intakeRoutingState) close() {
	if s.done != nil {
		close(s.done)
	}
	if s.reg != nil {
		s.reg.Close()
	}
}

// startIntakeRoutingLocked registers the endpoints of base that qualify for intake routing and
// waits, at most for the startup grace period, for their first decision. It returns the endpoints
// to start the pipeline with.
func (a *logAgent) startIntakeRoutingLocked(ctx context.Context, base *config.Endpoints) *config.Endpoints {
	reg, indices := a.registerIntakeRouting(base)
	if reg != nil {
		reg.WaitInitial(ctx)
	}
	a.routing = intakeRoutingState{base: base, reg: reg, indices: indices}
	return a.routedEndpointsLocked()
}

// registerIntakeRouting registers the endpoints of base that qualify for intake routing. It returns
// a nil registration when none does.
func (a *logAgent) registerIntakeRouting(base *config.Endpoints) (intakerouting.Registration, []int) {
	if a.intakeRouting == nil {
		return nil, nil
	}
	indices := base.LogsIntakeRoutingCandidates()
	if len(indices) == 0 {
		return nil, nil
	}
	reqs := make([]intakerouting.Request, len(indices))
	for i, idx := range indices {
		// The copy shares the endpoint's credential, so it sees API key rotations.
		endpoint := base.Endpoints[idx]
		reqs[i] = intakerouting.Request{
			Type:    intakerouting.Logs,
			Default: intakerouting.Endpoint{Host: endpoint.Host, Port: endpoint.Port},
			APIKey:  endpoint.GetAPIKeyIfReady,
		}
	}
	return a.intakeRouting.Register(reqs), indices
}

// routedEndpointsLocked returns the base endpoints, sent to the hosts currently chosen by intake
// routing.
func (a *logAgent) routedEndpointsLocked() *config.Endpoints {
	if a.routing.reg == nil {
		return a.routing.base
	}
	overrides := make(map[int]config.HostOverride)
	for i, current := range a.routing.reg.Current() {
		idx := a.routing.indices[i]
		configured := a.routing.base.Endpoints[idx]
		if current.Host != configured.Host || current.Port != configured.Port {
			overrides[idx] = config.HostOverride{Host: current.Host, Port: current.Port}
		}
	}
	return a.routing.base.WithHostOverrides(overrides)
}

// watchIntakeRoutingLocked restarts the pipeline whenever intake routing changes the hosts of the
// registered endpoints.
func (a *logAgent) watchIntakeRoutingLocked() {
	reg := a.routing.reg
	if reg == nil || a.routing.done != nil {
		return
	}
	done := make(chan struct{})
	a.routing.done = done
	handled := a.routingHandled
	go func() {
		for {
			select {
			case <-reg.Changes():
				a.onIntakeRoutingChange(reg)
				if handled != nil {
					handled(reg)
				}
			case <-done:
				return
			}
		}
	}()
}

// onIntakeRoutingChange restarts the pipeline on the hosts currently chosen by intake routing. If
// the restart fails, the pipeline rolls back to its previous endpoints and the next change retries.
func (a *logAgent) onIntakeRoutingChange(reg intakerouting.Registration) {
	a.restartMutex.Lock()
	defer a.restartMutex.Unlock()

	if a.stopped || a.routing.reg != reg || !a.endpoints.UseHTTP {
		return
	}
	routed := a.routedEndpointsLocked()
	if sameHosts(routed, a.endpoints) {
		return
	}
	a.log.Info("Intake routing changed the logs destinations, restarting the logs-agent pipeline")
	if err := a.restartLocked(routed); err != nil {
		a.log.Warnf("Could not restart the logs-agent pipeline on the hosts chosen by intake routing: %v", err)
	}
}

// closeIntakeRoutingLocked stops routing the endpoints.
func (a *logAgent) closeIntakeRoutingLocked() {
	a.routing.close()
	a.routing = intakeRoutingState{}
}

// sameHosts reports whether a and b send to the same hosts and ports.
func sameHosts(a, b *config.Endpoints) bool {
	if a.Main.Host != b.Main.Host || a.Main.Port != b.Main.Port || len(a.Endpoints) != len(b.Endpoints) {
		return false
	}
	for i := range a.Endpoints {
		if a.Endpoints[i].Host != b.Endpoints[i].Host || a.Endpoints[i].Port != b.Endpoints[i].Port {
			return false
		}
	}
	return true
}
