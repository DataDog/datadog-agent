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
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	kubeutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// markBatch marks the next batch of candidates, within the hourly cap.
func (c *Controller) markBatch(ctx context.Context, p *Policy, pc *pacing, exc *ExceptionSet, candidates []*appsv1.Deployment, touched map[types.UID]bool, now time.Time) error {
	left := min(pc.batch, pc.hourlyLeft(p, now))
	for _, d := range candidates {
		if left == 0 {
			return nil
		}
		marked, err := c.tryMark(ctx, p, exc, d, touched, now)
		if err != nil {
			return err
		}
		if marked {
			pc.members[d.UID] = true
			pc.changed = append(pc.changed, now)
			pc.inBatch = true
			left--
		}
	}
	return nil
}

// tryMark marks d with p, unless pre-flight or a dry run rejects it, or
// someone removed p's mark from it before. It reports whether it marked d.
func (c *Controller) tryMark(ctx context.Context, p *Policy, exc *ExceptionSet, d *appsv1.Deployment, touched map[types.UID]bool, now time.Time) (bool, error) {
	if gen, ok := c.rejected[rejectionKey{p.ID, d.UID}]; (ok && gen == d.Generation) || d.Spec.Paused || rolloutInProgress(d) {
		return false, nil
	}
	containers, reason := c.targets(p, exc, d)
	if len(containers) == 0 {
		c.reject(p, d, reason)
		return false, nil
	}
	marks := ParseMarks(d.Spec.Template.Annotations[PoliciesAnnotation])
	if _, unmarkedByLeader := marks[p.ID]; !unmarkedByLeader {
		history, err := c.replicaSets(ctx, d)
		if err != nil {
			return false, err
		}
		if prior := lastMarked(history, p.ID); len(prior) > 0 {
			return false, c.recordRemoval(ctx, p, d, prior, touched, now)
		}
	}
	containers, err := c.withoutCarried(ctx, p, d, containers)
	if err != nil || len(containers) == 0 {
		return false, err
	}
	marks[p.ID] = containers
	if reason, err := c.dryRun(ctx, p, d, marks, containers); reason != "" || err != nil {
		c.reject(p, d, reason)
		return false, err
	}
	if err := c.patchMarks(ctx, d, marks, nil); err != nil {
		return false, err
	}
	touched[d.UID] = true
	return true, nil
}

// reject reports d rejected for p until its spec changes. An empty reason
// means there is nothing to harden, which is not a rejection.
func (c *Controller) reject(p *Policy, d *appsv1.Deployment, reason string) {
	if reason == "" {
		return
	}
	c.rejected[rejectionKey{p.ID, d.UID}] = d.Generation
	c.report(p.ID, d, "rejected", reason)
}

// targets returns the containers of d that p should mark, or why d is
// rejected. Containers an exception, an emergency stop or an exclusion covers,
// or that set the control already, are skipped without a reason.
func (c *Controller) targets(p *Policy, exc *ExceptionSet, d *appsv1.Deployment) ([]string, string) {
	if d.Namespace == c.self.Namespace && (c.self.Name == "" || d.Name == c.self.Name) {
		return nil, ""
	}
	spec := &d.Spec.Template.Spec
	exclusions := parseExclusions(d.Annotations[ExcludedAnnotation])
	var targets, refused []string
	for _, ctr := range spec.Containers {
		switch {
		case exc.Exempts(d.Namespace, d.Name, ctr.Name, ctr.Image),
			exc.Stops(c.store.ClusterID(), d.Namespace, d.Name, ctr.Name, ctr.Image),
			excludes(exclusions, p.ID, ctr.Name),
			SetsControl(p.Control, ctr.Name, spec):
		case ctr.ReadinessProbe == nil && ctr.StartupProbe == nil:
			refused = append(refused, ctr.Name+": no readiness or startup probe, so pods that crash at start could replace every replica")
		default:
			if _, reason := Render(p.Control, ctr.Name, spec); reason != "" {
				refused = append(refused, ctr.Name+": "+reason)
				continue
			}
			targets = append(targets, ctr.Name)
		}
	}
	if len(targets) == 0 {
		return nil, strings.Join(refused, "; ")
	}
	if reason := deploymentSafety(d); reason != "" {
		return nil, reason
	}
	return targets, ""
}

// deploymentSafety reports why d's rollout cannot catch a control that breaks
// its pods, or "". Recreate tears down every replica first, and a
// maxUnavailable that covers every replica does the same in effect.
func deploymentSafety(d *appsv1.Deployment) string {
	if ptr.Deref(d.Spec.RevisionHistoryLimit, 1) == 0 {
		return "revisionHistoryLimit is 0, so a removed mark could not be told from a new Deployment"
	}
	if d.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
		return "deployment uses the Recreate strategy, so a failing control would take every pod down"
	}
	replicas := int(ptr.Deref(d.Spec.Replicas, 1))
	if replicas == 0 {
		return ""
	}
	maxUnavailable := ptr.To(intstr.FromString("25%"))
	if ru := d.Spec.Strategy.RollingUpdate; ru != nil && ru.MaxUnavailable != nil {
		maxUnavailable = ru.MaxUnavailable
	}
	// Round down, as the Deployment controller itself does.
	n, err := intstr.GetScaledValueFromIntOrPercent(maxUnavailable, replicas, false)
	if err != nil {
		return fmt.Sprintf("cannot resolve maxUnavailable: %v", err)
	}
	if n >= replicas {
		return "maxUnavailable allows every pod to be down"
	}
	return ""
}

// recordRemoval records that someone removed p's mark from d, so that no
// leader marks it again even once the ReplicaSet history is gone.
func (c *Controller) recordRemoval(ctx context.Context, p *Policy, d *appsv1.Deployment, containers []string, touched map[types.UID]bool, now time.Time) error {
	exclusions := append(parseExclusions(d.Annotations[ExcludedAnnotation]),
		Exclusion{Policy: p.ID, Containers: containers, Reason: ReasonRemoved, At: now})
	if err := c.patchMarks(ctx, d, ParseMarks(d.Spec.Template.Annotations[PoliciesAnnotation]), exclusions); err != nil {
		return err
	}
	touched[d.UID] = true
	c.report(p.ID, d, "removed", "an older ReplicaSet carries the policy but the pod template does not, so a rollback or a deploy tool removed it")
	return nil
}

// lastMarked returns the containers p marked in the newest template of
// history that p marks. Called for a template with no entry for p at all, a
// result means someone else removed the mark: the leader leaves an empty
// entry when it unmarks a Deployment itself, and records an exclusion when it
// reverts one.
func lastMarked(history []appsv1.ReplicaSet, policy string) []string {
	for i := len(history) - 1; i >= 0; i-- {
		if containers := ParseMarks(history[i].Spec.Template.Annotations[PoliciesAnnotation])[policy]; len(containers) > 0 {
			return containers
		}
	}
	return nil
}

// withoutCarried drops the containers whose running pods already carry the
// control, from another admission controller.
func (c *Controller) withoutCarried(ctx context.Context, p *Policy, d *appsv1.Deployment, containers []string) ([]string, error) {
	list, err := c.client.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector(d)})
	if err != nil {
		return nil, err
	}
	pods := slices.DeleteFunc(list.Items, func(pod corev1.Pod) bool { return !ownedBy(&pod, d) })
	if len(pods) == 0 {
		return containers, nil
	}
	return slices.DeleteFunc(containers, func(ctr string) bool {
		return !slices.ContainsFunc(pods, func(pod corev1.Pod) bool { return !SetsControl(p.Control, ctr, &pod.Spec) })
	}), nil
}

// dryRun dry-runs the template change to marks and the rendered
// securityContexts, and persists neither. It returns the reason when one is refused.
func (c *Controller) dryRun(ctx context.Context, p *Policy, d *appsv1.Deployment, marks Marks, containers []string) (string, error) {
	if err := c.patchMarksWith(ctx, d, marks, nil, true); err != nil {
		return refusal("dry run of the template change failed", err)
	}
	patched := make([]any, 0, len(containers))
	for _, ctr := range containers {
		sc, _ := Render(p.Control, ctr, &d.Spec.Template.Spec)
		patched = append(patched, map[string]any{"name": ctr, "securityContext": sc})
	}
	err := c.patch(ctx, d, types.StrategicMergePatchType, map[string]any{
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": patched}}},
	}, true)
	if err != nil {
		return refusal("dry run of the securityContext failed", err)
	}
	return "", nil
}

// refusal returns err as a rejection reason when the API server or the
// cluster's policy refused the patch: invalid, forbidden, or a bad request.
// Every other error says nothing about the Deployment, so it is returned for
// reconcile to retry.
func refusal(what string, err error) (string, error) {
	if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) || apierrors.IsBadRequest(err) {
		return fmt.Sprintf("%s: %v", what, err), nil
	}
	return "", err
}

// rolloutInProgress reports whether d has not finished rolling out its
// current template, including a Deployment that never finished its first.
func rolloutInProgress(d *appsv1.Deployment) bool {
	replicas := ptr.Deref(d.Spec.Replicas, 1)
	s := d.Status
	return s.ObservedGeneration < d.Generation ||
		s.UpdatedReplicas < replicas ||
		s.Replicas > s.UpdatedReplicas ||
		s.AvailableReplicas < s.UpdatedReplicas
}

// selector returns d's label selector; an invalid one selects everything,
// which the callers' owner checks then narrow.
func selector(d *appsv1.Deployment) string {
	sel, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		return ""
	}
	return sel.String()
}

// ownedBy reports whether pod belongs to d, through one of its ReplicaSets.
func ownedBy(pod *corev1.Pod, d *appsv1.Deployment) bool {
	ref := metav1.GetControllerOf(pod)
	return ref != nil && ref.Kind == "ReplicaSet" && kubeutil.ParseDeploymentForReplicaSet(ref.Name) == d.Name
}
