// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// EnabledLabel on a pod template routes its pods to the hardening webhook.
	EnabledLabel = "hardening.datadoghq.com/enabled"
	// PoliciesAnnotation on a pod template holds its Marks.
	PoliciesAnnotation = "hardening.datadoghq.com/policies"
	// AppliedAnnotation on a pod records what the webhook did for each
	// policy and container.
	AppliedAnnotation = "hardening.datadoghq.com/applied"
	// ExcludedAnnotation on a Deployment holds the Exclusions the leader recorded.
	ExcludedAnnotation = "hardening.datadoghq.com/excluded"
	// OptOutAnnotation set to "true" on a Deployment removes every policy
	// from it, for good. Only a person sets it.
	OptOutAnnotation = "hardening.datadoghq.com/opt-out"
	// AppliedUnknown is the AppliedAnnotation value for a policy the replica
	// serving the webhook has not loaded.
	AppliedUnknown = "skipped: unknown policy"
	// AppliedOK is the AppliedAnnotation value for a container the webhook hardened.
	AppliedOK = "applied"
)

// Marks maps a policy id to the containers it hardens in a pod template. A
// policy with no container is one the leader unmarked itself, which tells
// it apart from a mark someone else removed.
type Marks map[string][]string

// ParseMarks decodes a PoliciesAnnotation value; an invalid one has no marks.
func ParseMarks(v string) Marks {
	m := Marks{}
	if v != "" {
		_ = json.Unmarshal([]byte(v), &m)
	}
	return m
}

// hardening reports whether any policy hardens a container.
func (m Marks) hardening() bool {
	for _, containers := range m {
		if len(containers) > 0 {
			return true
		}
	}
	return false
}

func (m Marks) encode() string {
	raw, _ := json.Marshal(m)
	return string(raw)
}

// Exclusion records that a policy must not mark containers of a Deployment
// again: the leader reverted them (ReasonRolledBack), or someone removed the
// mark (ReasonRemoved). No container means every container.
type Exclusion struct {
	Policy     string    `json:"policy"`
	Containers []string  `json:"containers,omitempty"`
	Reason     string    `json:"reason"`
	At         time.Time `json:"at"`
}

const (
	// ReasonRolledBack is an Exclusion the leader recorded when it reverted a failed rollout.
	ReasonRolledBack = "rolled_back"
	// ReasonRemoved is an Exclusion the leader recorded when someone removed its mark.
	ReasonRemoved = "removed"
)

func parseExclusions(v string) []Exclusion {
	var list []Exclusion
	if v != "" {
		_ = json.Unmarshal([]byte(v), &list)
	}
	return list
}

func encodeExclusions(list []Exclusion) string {
	raw, _ := json.Marshal(list)
	return string(raw)
}

func excludes(list []Exclusion, policy, container string) bool {
	return slices.ContainsFunc(list, func(e Exclusion) bool {
		return e.Policy == policy && (len(e.Containers) == 0 || slices.Contains(e.Containers, container))
	})
}

// revertsSince counts the policy's reverts recorded after t.
func revertsSince(list []Exclusion, policy string, t time.Time) int {
	n := 0
	for _, e := range list {
		if e.Policy == policy && e.Reason == ReasonRolledBack && e.At.After(t) {
			n++
		}
	}
	return n
}

// AppliedKey is the AppliedAnnotation key of a policy on a container.
func AppliedKey(policy, container string) string {
	return policy + "/" + container
}

// appliedResults decodes a pod's AppliedAnnotation; a pod the webhook never saw has none.
func appliedResults(p *corev1.Pod) map[string]string {
	var results map[string]string
	_ = json.Unmarshal([]byte(p.Annotations[AppliedAnnotation]), &results)
	return results
}

// patchMarks sets d's template marks, with the label when one hardens a
// container, and, unless exclusions is nil, its exclusions.
func (c *Controller) patchMarks(ctx context.Context, d *appsv1.Deployment, marks Marks, exclusions []Exclusion) error {
	return c.patchMarksWith(ctx, d, marks, exclusions, false)
}

// patchMarksWith carries the resourceVersion the marks were read from, so a
// concurrent change fails with a conflict instead of being overwritten.
func (c *Controller) patchMarksWith(ctx context.Context, d *appsv1.Deployment, marks Marks, exclusions []Exclusion, dryRun bool) error {
	var label, annotation any // null removes the key in a merge patch
	if marks.hardening() {
		label = "true"
	}
	if len(marks) > 0 {
		annotation = marks.encode()
	}
	metadata := map[string]any{"resourceVersion": d.ResourceVersion}
	if exclusions != nil {
		metadata["annotations"] = map[string]any{ExcludedAnnotation: encodeExclusions(exclusions)}
	}
	return c.patch(ctx, d, types.MergePatchType, map[string]any{
		"metadata": metadata,
		"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{
			"labels":      map[string]any{EnabledLabel: label},
			"annotations": map[string]any{PoliciesAnnotation: annotation},
		}}},
	}, dryRun)
}

func (c *Controller) patch(ctx context.Context, d *appsv1.Deployment, pt types.PatchType, body any, dryRun bool) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	opts := metav1.PatchOptions{FieldManager: fieldManager}
	if dryRun {
		opts.DryRun = []string{metav1.DryRunAll}
	}
	_, err = c.client.AppsV1().Deployments(d.Namespace).Patch(ctx, d.Name, pt, data, opts)
	return err
}
