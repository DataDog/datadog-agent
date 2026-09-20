// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"encoding/json"
	"testing"
	"time"
)

// Test partitions:
// - flip state: first write | same value | flip within hold | flip after hold | drop after rule deletion
// - boundary: exactly at the hold interval

// TestFlipHold covers: first write immediate; unchanged value is a no-op; flip inside the hold is held; flip after the hold passes.
func TestFlipHold(t *testing.T) {
	hold := newFlipHold()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	value, changed := hold.resolve("pod/ns/pod-a", "is_leader", "true", 15*time.Second, now)
	if value != "true" || !changed {
		t.Errorf("first write: got (%q, %v), want (\"true\", true)", value, changed)
	}

	value, changed = hold.resolve("pod/ns/pod-a", "is_leader", "true", 15*time.Second, now.Add(5*time.Second))
	if value != "true" || changed {
		t.Errorf("same value: got (%q, %v), want (\"true\", false)", value, changed)
	}

	// Flip inside the hold: previous value is kept.
	value, changed = hold.resolve("pod/ns/pod-a", "is_leader", "false", 15*time.Second, now.Add(10*time.Second))
	if value != "true" || changed {
		t.Errorf("flip inside hold: got (%q, %v), want (\"true\", false)", value, changed)
	}

	// Boundary: exactly one hold after the write, the flip passes.
	value, changed = hold.resolve("pod/ns/pod-a", "is_leader", "false", 15*time.Second, now.Add(15*time.Second))
	if value != "false" || !changed {
		t.Errorf("flip at hold boundary: got (%q, %v), want (\"false\", true)", value, changed)
	}
}

// TestFlipHoldDropTagKey covers: drop releases state so a later write initializes fresh.
func TestFlipHoldDropTagKey(t *testing.T) {
	hold := newFlipHold()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	hold.resolve("pod/ns/pod-a", "is_leader", "true", 15*time.Second, now)
	hold.resolve("pod/ns/pod-b", "is_leader", "true", 15*time.Second, now)
	hold.resolve("pod/ns/pod-a", "owning_team", "frontend", 15*time.Second, now)

	hold.dropTagKey("is_leader")

	// After dropping the tag key, the next write initializes fresh: the flip
	// is immediate, not held.
	value, changed := hold.resolve("pod/ns/pod-a", "is_leader", "false", 15*time.Second, now.Add(time.Second))
	if value != "false" || !changed {
		t.Errorf("write after dropTagKey: got (%q, %v), want (\"false\", true)", value, changed)
	}

	// Other tag keys keep their state.
	value, _ = hold.resolve("pod/ns/pod-a", "owning_team", "frontend", 15*time.Second, now.Add(time.Second))
	if value != "frontend" {
		t.Errorf("other tag key after dropTagKey: got %q, want %q", value, "frontend")
	}
}

// TestComputePodAnnotations covers: fresh pod gets owned keys on every container + ledger;
// user keys preserved with drift overwritten; stale owned keys stripped; unparseable JSON untouched.
func TestComputePodAnnotations(t *testing.T) {
	ownedIsLeader := map[string]string{"is_leader": "true"}

	t.Run("fresh pod", func(t *testing.T) {
		pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", nil)
		setContainers(pod, "app", "istio-proxy")

		patch, err := computePodAnnotations(pod, ownedIsLeader)
		if err != nil {
			t.Fatalf("computePodAnnotations: %v", err)
		}
		for _, container := range []string{"app", "istio-proxy"} {
			raw, ok := patch[adTagsAnnotationKey(container)]
			if !ok || raw == nil {
				t.Fatalf("container %s: missing tags annotation in patch", container)
			}
			var tags map[string]string
			if err := json.Unmarshal([]byte(*raw), &tags); err != nil {
				t.Fatalf("container %s: bad JSON: %v", container, err)
			}
			if tags["is_leader"] != "true" {
				t.Errorf("container %s: got %v, want is_leader=true", container, tags)
			}
		}
		ledger, ok := patch[ManagedTagKeysAnnotation]
		if !ok || ledger == nil || *ledger != "is_leader" {
			t.Errorf("ledger: got %v, want is_leader", patch[ManagedTagKeysAnnotation])
		}
	})

	t.Run("user keys preserved, drift overwritten", func(t *testing.T) {
		pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", nil)
		setContainers(pod, "app")
		setAnnotations(pod, map[string]string{
			adTagsAnnotationKey("app"): `{"team":"core","is_leader":"false"}`,
			ManagedTagKeysAnnotation:   "is_leader",
		})

		patch, err := computePodAnnotations(pod, ownedIsLeader)
		if err != nil {
			t.Fatalf("computePodAnnotations: %v", err)
		}
		raw := patch[adTagsAnnotationKey("app")]
		if raw == nil {
			t.Fatal("app annotation missing from patch")
		}
		var tags map[string]string
		if err := json.Unmarshal([]byte(*raw), &tags); err != nil {
			t.Fatalf("bad JSON: %v", err)
		}
		if tags["team"] != "core" {
			t.Errorf("user key: got %q, want preserved", tags["team"])
		}
		if tags["is_leader"] != "true" {
			t.Errorf("drifted owned key: got %q, want restored to true", tags["is_leader"])
		}
	})

	t.Run("stale owned keys stripped", func(t *testing.T) {
		pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", nil)
		setContainers(pod, "app")
		setAnnotations(pod, map[string]string{
			adTagsAnnotationKey("app"): `{"team":"core","is_leader":"true","is_schedulable":"true"}`,
			ManagedTagKeysAnnotation:   "is_leader,is_schedulable",
		})

		// is_schedulable's rule no longer matches: only is_leader stays.
		patch, err := computePodAnnotations(pod, ownedIsLeader)
		if err != nil {
			t.Fatalf("computePodAnnotations: %v", err)
		}
		raw := patch[adTagsAnnotationKey("app")]
		var tags map[string]string
		if err := json.Unmarshal([]byte(*raw), &tags); err != nil {
			t.Fatalf("bad JSON: %v", err)
		}
		if _, ok := tags["is_schedulable"]; ok {
			t.Errorf("stale key: still present in %v", tags)
		}
		if tags["is_leader"] != "true" {
			t.Errorf("owned key: got %v, want kept", tags)
		}
	})

	t.Run("unparseable user JSON untouched", func(t *testing.T) {
		pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", nil)
		setContainers(pod, "app")
		setAnnotations(pod, map[string]string{adTagsAnnotationKey("app"): "not json"})

		patch, err := computePodAnnotations(pod, ownedIsLeader)
		if err != nil {
			t.Fatalf("computePodAnnotations: %v", err)
		}
		if _, touched := patch[adTagsAnnotationKey("app")]; touched {
			t.Errorf("unparseable annotation was touched: %v", patch)
		}
	})

	t.Run("all owned keys gone removes annotation and ledger", func(t *testing.T) {
		pod := newEntity(EntityPod, "ben-bitdiddle", "my-namespace", nil)
		setContainers(pod, "app")
		setAnnotations(pod, map[string]string{
			adTagsAnnotationKey("app"): `{"is_leader":"true"}`,
			ManagedTagKeysAnnotation:   "is_leader",
		})

		patch, err := computePodAnnotations(pod, map[string]string{})
		if err != nil {
			t.Fatalf("computePodAnnotations: %v", err)
		}
		if v, ok := patch[adTagsAnnotationKey("app")]; !ok || v != nil {
			t.Errorf("app annotation: got %v, want delete", patch[adTagsAnnotationKey("app")])
		}
		if v, ok := patch[ManagedTagKeysAnnotation]; !ok || v != nil {
			t.Errorf("ledger: got %v, want delete", patch[ManagedTagKeysAnnotation])
		}
	})
}

// TestComputeNodeAnnotations covers: fresh node writes prefix keys + ledger; stale keys deleted; empty owned drops ledger.
func TestComputeNodeAnnotations(t *testing.T) {
	node := newEntity(EntityNode, "node-a", "", nil)
	setAnnotations(node, map[string]string{
		NodeTagAnnotationPrefix + "is_schedulable": "true",
		ManagedTagKeysAnnotation:                   "is_schedulable",
	})

	// Rule still owns the key plus a new one.
	patch, err := computeNodeAnnotations(node, map[string]string{"is_schedulable": "false", "owning_team": "infra"})
	if err != nil {
		t.Fatalf("computeNodeAnnotations: %v", err)
	}
	if got := *patch[NodeTagAnnotationPrefix+"is_schedulable"]; got != "false" {
		t.Errorf("is_schedulable: got %q, want false", got)
	}
	if got := *patch[NodeTagAnnotationPrefix+"owning_team"]; got != "infra" {
		t.Errorf("owning_team: got %q, want infra", got)
	}
	if got := *patch[ManagedTagKeysAnnotation]; got != "is_schedulable,owning_team" {
		t.Errorf("ledger: got %q, want both keys", got)
	}

	// Only a stale key remains owned by nothing: prefix key and ledger are deleted.
	setAnnotations(node, map[string]string{
		NodeTagAnnotationPrefix + "is_schedulable": "true",
		NodeTagAnnotationPrefix + "owning_team":    "infra",
		ManagedTagKeysAnnotation:                   "is_schedulable,owning_team",
	})
	patch, err = computeNodeAnnotations(node, map[string]string{})
	if err != nil {
		t.Fatalf("computeNodeAnnotations empty: %v", err)
	}
	for key, want := range map[string]*string{
		NodeTagAnnotationPrefix + "is_schedulable": nil,
		NodeTagAnnotationPrefix + "owning_team":    nil,
		ManagedTagKeysAnnotation:                   nil,
	} {
		if got, ok := patch[key]; !ok || got != want {
			t.Errorf("key %s: got %v, want delete", key, got)
		}
	}
}
