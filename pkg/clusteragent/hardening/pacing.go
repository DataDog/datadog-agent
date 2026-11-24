// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"fmt"
	"slices"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// pacing is how fast the leader changes one policy's Deployments in this cluster.
type pacing struct {
	// batch is how many Deployments the next batch marks.
	batch int
	// members are the Deployments of the current batch whose rollout has
	// not finished with the control on every pod.
	members map[types.UID]bool
	// inBatch is set from the start of a batch until its members settle.
	inBatch bool
	// reverted is set when the leader reverted a Deployment during the batch.
	reverted bool
	// changed holds when the leader changed a template for the policy, over the last hour.
	changed []time.Time
}

// passSummary is what one pass over a policy's marked Deployments found.
type passSummary struct {
	// degraded describes the first Deployment that failed after its rollout
	// succeeded and after the cluster was added, or is empty.
	degraded string
	// reverts counts the rollouts reverted during the pass, which the
	// Deployments it started from do not record yet.
	reverts int
}

func (c *Controller) pacingFor(p *Policy) *pacing {
	pc, ok := c.pacing[p.ID]
	if !ok {
		pc = &pacing{batch: p.Limits.FirstBatch, members: map[types.UID]bool{}}
		c.pacing[p.ID] = pc
	}
	return pc
}

// hourlyLeft returns how many more templates the leader may change this hour.
func (pc *pacing) hourlyLeft(p *Policy, now time.Time) int {
	pc.changed = slices.DeleteFunc(pc.changed, func(t time.Time) bool { return now.Sub(t) >= time.Hour })
	return max(0, p.Limits.HourlyCap-len(pc.changed))
}

// onRevert sends the leader back to small batches.
func (pc *pacing) onRevert(p *Policy, d *appsv1.Deployment) {
	pc.reverted = true
	pc.batch = p.Limits.FirstBatch
	delete(pc.members, d.UID)
}

// settle drops the batch members whose rollout finished with the control on
// every pod, or that the policy no longer marks. Once none is left, the next
// batch grows, unless one was reverted.
func (c *Controller) settle(p *Policy, pc *pacing, deployments []*appsv1.Deployment) {
	for _, d := range deployments {
		if !pc.members[d.UID] {
			continue
		}
		containers := ParseMarks(d.Spec.Template.Annotations[PoliciesAnnotation])[p.ID]
		if len(containers) == 0 || (!rolloutInProgress(d) && c.unhardened(d, p, containers) == "") {
			delete(pc.members, d.UID)
		}
	}
	present := map[types.UID]bool{}
	for _, d := range deployments {
		present[d.UID] = true
	}
	for uid := range pc.members {
		if !present[uid] {
			delete(pc.members, uid)
		}
	}
	// ponytail: a member whose pods never get the control (the webhook was
	// down during its rollout) holds the batch until its next rollout; add a
	// timeout if that shows up.
	if !pc.inBatch || len(pc.members) > 0 {
		return
	}
	if !pc.reverted {
		pc.batch = min(2*pc.batch, p.Limits.HourlyCap)
	}
	pc.inBatch, pc.reverted = false, false
}

// markWait returns why the leader marks no new Deployment for p now, or "".
func (c *Controller) markWait(p *Policy, pc *pacing, ready bool, sum *passSummary, deployments, candidates []*appsv1.Deployment, now time.Time) string {
	if len(candidates) == 0 {
		c.setPaused(p, "")
		return "no Deployment left to mark"
	}
	paused := pauseReason(p, deployments, sum)
	c.setPaused(p, paused)
	switch {
	case paused != "":
		return "paused: " + paused
	case !ready:
		return fmt.Sprintf("every shard of an exceptions generation of at least %d", p.MinExceptionsGeneration)
	case now.Sub(c.firstSeen[p.ID]) < c.syncWait:
		return "every Cluster Agent replica to load the policy"
	case len(pc.members) > 0:
		return "the current batch to finish its rollouts"
	case pc.hourlyLeft(p, now) == 0:
		return "the hourly cap"
	}
	return ""
}

// pauseReason returns why the cluster is paused for p, or "". Only reverts
// and failures since the team last added the cluster count, so re-adding a
// paused cluster resumes it.
func pauseReason(p *Policy, deployments []*appsv1.Deployment, sum *passSummary) string {
	reverts := sum.reverts
	for _, d := range deployments {
		reverts += revertsSince(parseExclusions(d.Annotations[ExcludedAnnotation]), p.ID, p.cluster.AddedAt)
	}
	if reverts >= p.Limits.PauseThreshold {
		return fmt.Sprintf("%d rollouts were reverted since the cluster was added at %s", reverts, p.cluster.AddedAt.Format(time.RFC3339))
	}
	return sum.degraded
}
