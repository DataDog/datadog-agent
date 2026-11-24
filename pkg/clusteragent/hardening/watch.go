// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// watch follows a Deployment p marks. It removes the containers an emergency
// stop covers at once, and those an exception covers at the usual pace. It
// reverts a rollout the leader started that failed, and reports any later
// failure as degraded.
func (c *Controller) watch(ctx context.Context, p *Policy, pc *pacing, exc *ExceptionSet, d *appsv1.Deployment, containers []string, touched map[types.UID]bool, sum *passSummary, now time.Time) error {
	if stopped := covered(containers, d, func(ctr *corev1.Container) bool {
		return exc != nil && exc.Stops(c.store.ClusterID(), d.Namespace, d.Name, ctr.Name, ctr.Image)
	}); len(stopped) > 0 {
		return c.exempt(ctx, p, d, containers, stopped, touched, "an emergency stop")
	}
	// Before failures: exempting a degraded Deployment is how a team clears its pause.
	if exempt := covered(containers, d, func(ctr *corev1.Container) bool {
		return exc != nil && exc.Exempts(d.Namespace, d.Name, ctr.Name, ctr.Image)
	}); len(exempt) > 0 && pc.hourlyLeft(p, now) > 0 {
		pc.changed = append(pc.changed, now)
		return c.exempt(ctx, p, d, containers, exempt, touched, "an exception")
	}
	if problem, at := c.failure(d, p, containers); problem != "" {
		return c.onFailure(ctx, p, pc, d, containers, problem, at, touched, sum, now)
	}
	if !rolloutInProgress(d) && c.unhardened(d, p, containers) == "" {
		c.report(p.ID, d, "applied", "")
	}
	return nil
}

// covered returns the marked containers of d's template that match.
func covered(containers []string, d *appsv1.Deployment, match func(*corev1.Container) bool) []string {
	return slices.DeleteFunc(slices.Clone(containers), func(name string) bool {
		ctr := FindContainer(&d.Spec.Template.Spec, name)
		return ctr == nil || !match(ctr)
	})
}

// exempt removes containers from p's mark on d, leaving an empty entry when
// none is left, so the removal is told apart from someone else's.
func (c *Controller) exempt(ctx context.Context, p *Policy, d *appsv1.Deployment, containers, removed []string, touched map[types.UID]bool, by string) error {
	marks := ParseMarks(d.Spec.Template.Annotations[PoliciesAnnotation])
	marks[p.ID] = slices.DeleteFunc(slices.Clone(containers), func(ctr string) bool { return slices.Contains(removed, ctr) })
	if err := c.patchMarks(ctx, d, marks, nil); err != nil {
		return err
	}
	touched[d.UID] = true
	c.report(p.ID, d, "exempted", fmt.Sprintf("%s covers %v", by, removed))
	return nil
}

// onFailure reverts d if the failing rollout is the one the leader started
// and has not succeeded yet. Otherwise only a person removes the control, so
// it reports d degraded, which pauses the cluster if it failed since the team
// added it.
func (c *Controller) onFailure(ctx context.Context, p *Policy, pc *pacing, d *appsv1.Deployment, containers []string, problem string, at time.Time, touched map[types.UID]bool, sum *passSummary, now time.Time) error {
	if !succeeded(d) {
		history, err := c.replicaSets(ctx, d)
		if err != nil {
			return err
		}
		if startedByLeader(history, d) {
			pc.onRevert(p, d)
			sum.reverts++
			return c.revert(ctx, p, d, containers, problem, touched, now)
		}
	}
	c.report(p.ID, d, "degraded", problem)
	if sum.degraded == "" && at.After(p.cluster.AddedAt) {
		sum.degraded = fmt.Sprintf("%s is degraded: %s", key(d), problem)
	}
	return nil
}

// revert removes p's mark from d, which rolls it back to pods without the
// control, and excludes the containers from p in this cluster.
func (c *Controller) revert(ctx context.Context, p *Policy, d *appsv1.Deployment, containers []string, problem string, touched map[types.UID]bool, now time.Time) error {
	marks := ParseMarks(d.Spec.Template.Annotations[PoliciesAnnotation])
	delete(marks, p.ID)
	exclusions := append(parseExclusions(d.Annotations[ExcludedAnnotation]),
		Exclusion{Policy: p.ID, Containers: containers, Reason: ReasonRolledBack, At: now})
	if err := c.patchMarks(ctx, d, marks, exclusions); err != nil {
		return err
	}
	touched[d.UID] = true
	c.report(p.ID, d, "rolled_back", problem)
	return nil
}

// unmarkInactive removes a stopped policy, or one that no longer lists this
// cluster, from the Deployments it marks, at the usual pace.
func (c *Controller) unmarkInactive(ctx context.Context, p *Policy, pc *pacing, deployments []*appsv1.Deployment, touched map[types.UID]bool, now time.Time) error {
	for _, d := range deployments {
		marks := ParseMarks(d.Spec.Template.Annotations[PoliciesAnnotation])
		if touched[d.UID] || len(marks[p.ID]) == 0 {
			continue
		}
		if pc.hourlyLeft(p, now) == 0 {
			return nil
		}
		marks[p.ID] = []string{}
		if err := c.patchMarks(ctx, d, marks, nil); err != nil {
			return err
		}
		pc.changed = append(pc.changed, now)
		touched[d.UID] = true
		c.clearStatus(p.ID, d)
	}
	return nil
}

// succeeded reports whether d's latest rollout completed. The Deployment
// controller keeps that condition when pods fail later, until the next rollout.
func succeeded(d *appsv1.Deployment) bool {
	if d.Status.ObservedGeneration < d.Generation {
		return false
	}
	return slices.ContainsFunc(d.Status.Conditions, func(c appsv1.DeploymentCondition) bool {
		return c.Type == appsv1.DeploymentProgressing && c.Reason == "NewReplicaSetAvailable"
	})
}

// startedByLeader reports whether d's current rollout comes from a template
// change to the hardening keys alone, which only the leader makes.
func startedByLeader(history []appsv1.ReplicaSet, d *appsv1.Deployment) bool {
	cur := slices.IndexFunc(history, func(rs appsv1.ReplicaSet) bool {
		return sameTemplate(&rs.Spec.Template, &d.Spec.Template, false)
	})
	return cur > 0 && sameTemplate(&history[cur-1].Spec.Template, &d.Spec.Template, true)
}

// failure returns the first sign that the pods p hardens on d fail, and when
// it happened, or "".
func (c *Controller) failure(d *appsv1.Deployment, p *Policy, containers []string) (string, time.Time) {
	if d.Status.ObservedGeneration >= d.Generation {
		for _, cond := range d.Status.Conditions {
			switch {
			case cond.Type == appsv1.DeploymentProgressing && cond.Reason == "ProgressDeadlineExceeded":
				return "the rollout stalled (ProgressDeadlineExceeded)", cond.LastUpdateTime.Time
			case cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionFalse:
				return "the Deployment lost availability (Available=False)", cond.LastTransitionTime.Time
			}
		}
	}
	for _, pod := range c.podsOf(d) {
		results := appliedResults(pod)
		for _, s := range pod.Status.ContainerStatuses {
			t := s.LastTerminationState.Terminated
			// An OOM kill says nothing about the control.
			if !slices.Contains(containers, s.Name) || results[AppliedKey(p.ID, s.Name)] != AppliedOK || s.RestartCount == 0 || t == nil || t.Reason == "OOMKilled" {
				continue
			}
			return fmt.Sprintf("container %s of pod %s restarted (%s, exit code %d)", s.Name, pod.Name, t.Reason, t.ExitCode), t.FinishedAt.Time
		}
	}
	return "", time.Time{}
}

// unhardened returns why a pod of p's marked template on d runs without the
// control, or "".
func (c *Controller) unhardened(d *appsv1.Deployment, p *Policy, containers []string) string {
	seen := false
	for _, pod := range c.podsOf(d) {
		if len(ParseMarks(pod.Annotations[PoliciesAnnotation])[p.ID]) == 0 {
			continue // a pod of an older template
		}
		seen = true
		results := appliedResults(pod)
		for _, ctr := range containers {
			if r := results[AppliedKey(p.ID, ctr)]; r != AppliedOK {
				return fmt.Sprintf("pod %s started without the control on %s (%q)", pod.Name, ctr, r)
			}
		}
	}
	if !seen && ptr.Deref(d.Spec.Replicas, 1) > 0 {
		return "no pod of the marked template yet"
	}
	return ""
}

// podsOf returns d's pods of marked templates, from the informer.
func (c *Controller) podsOf(d *appsv1.Deployment) []*corev1.Pod {
	sel, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		sel = labels.Everything()
	}
	pods, _ := c.pods.Pods(d.Namespace).List(sel)
	return slices.DeleteFunc(pods, func(pod *corev1.Pod) bool { return !ownedBy(pod, d) })
}

// replicaSets returns d's ReplicaSets, oldest revision first: its history.
func (c *Controller) replicaSets(ctx context.Context, d *appsv1.Deployment) ([]appsv1.ReplicaSet, error) {
	list, err := c.client.AppsV1().ReplicaSets(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector(d)})
	if err != nil {
		return nil, err
	}
	history := slices.DeleteFunc(list.Items, func(rs appsv1.ReplicaSet) bool {
		ref := metav1.GetControllerOf(&rs)
		return ref == nil || ref.UID != d.UID
	})
	sort.SliceStable(history, func(i, j int) bool { return revision(&history[i]) < revision(&history[j]) })
	return history, nil
}

// revision returns the Deployment revision a ReplicaSet runs; a rollback
// gives an older ReplicaSet the newest one.
func revision(rs *appsv1.ReplicaSet) int64 {
	n, _ := strconv.ParseInt(rs.Annotations["deployment.kubernetes.io/revision"], 10, 64)
	return n
}

// sameTemplate reports whether two pod templates are the same, apart from a
// ReplicaSet's pod-template-hash label and, with ignoreHardening, the
// hardening label and marks.
func sameTemplate(a, b *corev1.PodTemplateSpec, ignoreHardening bool) bool {
	strip := func(t *corev1.PodTemplateSpec) *corev1.PodTemplateSpec {
		t = t.DeepCopy()
		delete(t.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
		if ignoreHardening {
			delete(t.Labels, EnabledLabel)
			delete(t.Annotations, PoliciesAnnotation)
		}
		if len(t.Labels) == 0 {
			t.Labels = nil
		}
		if len(t.Annotations) == 0 {
			t.Annotations = nil
		}
		return t
	}
	return equality.Semantic.DeepEqual(strip(a), strip(b))
}
