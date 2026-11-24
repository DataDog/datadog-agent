// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// statusKey is a policy and a Deployment, or the cluster when deployment is empty.
type statusKey struct {
	policy     string
	deployment string
}

// report records one of the Deployment statuses the RFC defines (rejected,
// applied, rolled_back, degraded, removed, exempted), and logs it when it changed.
// ponytail: logs only, until the status events have an Event Platform track.
func (c *Controller) report(policy string, d *appsv1.Deployment, status, reason string) {
	c.record(statusKey{policy, key(d)}, status, reason)
}

// setPaused reports the cluster paused for p, or resumed once reason is empty.
func (c *Controller) setPaused(p *Policy, reason string) {
	k := statusKey{policy: p.ID}
	if reason == "" {
		if _, ok := c.status[k]; ok {
			delete(c.status, k)
			emit(k, "resumed")
		}
		return
	}
	c.record(k, "paused", reason)
}

func (c *Controller) record(k statusKey, status, reason string) {
	if reason != "" {
		status += ": " + reason
	}
	if c.status[k] == status {
		return
	}
	c.status[k] = status
	emit(k, status)
}

func (c *Controller) clearStatus(policy string, d *appsv1.Deployment) {
	delete(c.status, statusKey{policy, key(d)})
}

func emit(k statusKey, status string) {
	if k.deployment == "" {
		log.Infof("hardening policy %s: cluster %s", k.policy, status)
		return
	}
	log.Infof("hardening policy %s on %s: %s", k.policy, k.deployment, status)
}

// heartbeat reports every status again each heartbeatPeriod.
func (c *Controller) heartbeat(now time.Time) {
	if now.Sub(c.lastHeartbeat) < heartbeatPeriod {
		return
	}
	c.lastHeartbeat = now
	for k, status := range c.status {
		emit(k, status)
	}
}
