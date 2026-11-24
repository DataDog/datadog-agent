// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const testCluster = "cluster-1"

// rawRequest returns a valid request as JSON, after mutate edits its fields.
func rawRequest(t *testing.T, mutate func(m map[string]any)) []byte {
	m := map[string]any{
		"id":      "req-1",
		"action":  "enforce",
		"control": "capabilities",
		"workload": map[string]any{
			"kind": "Deployment", "namespace": "shop", "name": "web", "container": "app",
		},
		"parameters":  map[string]any{"capabilities_add": []string{"cap_net_bind_service"}},
		"clusters":    []any{map[string]any{"id": testCluster, "added_at": time.Now().Add(-time.Minute).Format(time.RFC3339)}},
		"confirm_for": "1h",
	}
	if mutate != nil {
		mutate(m)
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

func workload(m map[string]any) map[string]any { return m["workload"].(map[string]any) }

func TestParseRequest(t *testing.T) {
	r, err := parseRequest(rawRequest(t, nil), testCluster)
	require.NoError(t, err)
	assert.Equal(t, "req-1", r.ID)
	assert.Equal(t, []string{"NET_BIND_SERVICE"}, r.Parameters.CapabilitiesAdd)
	assert.True(t, r.Listed())
	assert.Equal(t, time.Hour, r.confirmFor)
	assert.NotEmpty(t, r.hash)

	invalid := map[string]func(m map[string]any){
		"unknown field":        func(m map[string]any) { m["patch"] = "{}" },
		"id with comma":        func(m map[string]any) { m["id"] = "a,b" },
		"empty id":             func(m map[string]any) { m["id"] = "" },
		"unknown action":       func(m map[string]any) { m["action"] = "trial" },
		"unknown control":      func(m map[string]any) { m["control"] = "seccomp_runtime_default" },
		"capabilities on ro":   func(m map[string]any) { m["control"] = "read_only_root_fs" },
		"capabilities on h2":   func(m map[string]any) { m["control"] = "non_root" },
		"capability ALL":       func(m map[string]any) { m["parameters"] = map[string]any{"capabilities_add": []string{"ALL"}} },
		"malformed capability": func(m map[string]any) { m["parameters"] = map[string]any{"capabilities_add": []string{"NET BIND"}} },
		"unknown capability":   func(m map[string]any) { m["parameters"] = map[string]any{"capabilities_add": []string{"FOO"}} },
		"statefulset":          func(m map[string]any) { workload(m)["kind"] = "StatefulSet" },
		"cluster in workload":  func(m map[string]any) { workload(m)["cluster"] = testCluster },
		"image_id":             func(m map[string]any) { workload(m)["image_id"] = "web@sha256:abc" },
		"missing container":    func(m map[string]any) { delete(workload(m), "container") },
		"cluster without id":   func(m map[string]any) { m["clusters"] = []any{map[string]any{"added_at": "2026-11-04T15:00:00Z"}} },
		"cluster without date": func(m map[string]any) { m["clusters"] = []any{map[string]any{"id": testCluster}} },
		"bad confirm_for":      func(m map[string]any) { m["confirm_for"] = "7d" },
		"negative confirm_for": func(m map[string]any) { m["confirm_for"] = "-1h" },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := parseRequest(rawRequest(t, mutate), testCluster)
			assert.Error(t, err)
		})
	}
}

func TestParseRequestDefaultsConfirmFor(t *testing.T) {
	r, err := parseRequest(rawRequest(t, func(m map[string]any) { delete(m, "confirm_for") }), testCluster)
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, r.confirmFor)
}

func TestParseRequestKeepsUnlistedCluster(t *testing.T) {
	r, err := parseRequest(rawRequest(t, nil), "cluster-2")
	require.NoError(t, err, "a cluster that left the list still needs the request to remove it")
	assert.False(t, r.Listed())
	assert.False(t, r.Active(time.Now()))

	r, err = parseRequest(rawRequest(t, func(m map[string]any) { m["clusters"] = []any{} }), testCluster)
	require.NoError(t, err)
	assert.False(t, r.Listed())
}

func TestParseRequestHashIgnoresOtherClusters(t *testing.T) {
	r1, err := parseRequest(rawRequest(t, nil), testCluster)
	require.NoError(t, err)
	r2, err := parseRequest(rawRequest(t, func(m map[string]any) {
		m["clusters"] = append(m["clusters"].([]any), map[string]any{"id": "cluster-2", "added_at": "2026-11-12T16:00:00Z"})
	}), testCluster)
	require.NoError(t, err)
	assert.Equal(t, r1.hash, r2.hash, "promoting the request to another cluster changes nothing here")

	r3, err := parseRequest(rawRequest(t, func(m map[string]any) { m["clusters"] = []any{} }), testCluster)
	require.NoError(t, err)
	assert.NotEqual(t, r1.hash, r3.hash, "leaving the list does")
}

func TestParseRequestValidate(t *testing.T) {
	r, err := parseRequest(rawRequest(t, func(m map[string]any) { m["action"] = "validate" }), testCluster)
	require.NoError(t, err)
	assert.False(t, r.Active(time.Now()), "a validate request is never applied")
}

// TestParseRequestRFCExample parses Appendix E's H1 example verbatim.
func TestParseRequestRFCExample(t *testing.T) {
	raw := `{
		"id": "hr-01JBA7K3Q2M8N4P6R9T1V5X7Z0",
		"action": "enforce",
		"control": "capabilities",
		"workload": {
			"kind": "Deployment",
			"namespace": "payments",
			"name": "checkout-api",
			"container": "api"
		},
		"clusters": [
			{"id": "3b6f2c1e-9d4a-4f7e-8c25-1a0e7d9b5f42", "added_at": "2026-11-04T15:00:00Z"},
			{"id": "8d0a4e7b-2c1f-4b9e-a6d3-5f7c9e1b2a40", "added_at": "2026-11-04T15:00:00Z"},
			{"id": "c41e9b2d-7a3f-4e8c-9b1d-2e6f8a0c5d17", "added_at": "2026-11-12T16:00:00Z"}
		],
		"parameters": {"capabilities_add": ["NET_BIND_SERVICE"]},
		"confirm_for": "168h"
	}`
	r, err := parseRequest([]byte(raw), "c41e9b2d-7a3f-4e8c-9b1d-2e6f8a0c5d17")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC), r.addedAt)
	assert.True(t, r.Active(time.Now()))
}

// TestParseRequestEmptyClusterNeverMatches guards against an unconfigured
// (empty) Cluster Agent clusterID matching a cluster entry with no id.
func TestParseRequestEmptyClusterNeverMatches(t *testing.T) {
	raw := rawRequest(t, func(m map[string]any) {
		m["clusters"] = []any{map[string]any{"id": "", "added_at": "2026-11-04T15:00:00Z"}}
	})
	_, err := parseRequest(raw, "")
	assert.Error(t, err)
}

func TestRequestActive(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	listed := func(r Request) *Request { r.addedAt = past; return &r }
	assert.True(t, listed(Request{Action: ActionEnforce}).Active(now), "no expiry")
	assert.True(t, listed(Request{Action: ActionEnforce, ExpiresAt: &future}).Active(now))
	assert.False(t, listed(Request{Action: ActionEnforce, ExpiresAt: &past}).Active(now))
	assert.False(t, listed(Request{Action: ActionRevert}).Active(now))
	assert.False(t, (&Request{Action: ActionEnforce}).Active(now), "not listed")
}

func TestStoreRCUpdate(t *testing.T) {
	s := NewStore(testCluster)
	statuses := map[string]state.ApplyStatus{}
	ack := func(path string, st state.ApplyStatus) { statuses[path] = st }

	s.OnRCUpdate(map[string]state.RawConfig{
		"path/good": {Config: rawRequest(t, nil)},
		"path/bad":  {Config: []byte(`{"id":`)},
	}, ack)

	assert.Equal(t, state.ApplyStateAcknowledged, statuses["path/good"].State)
	assert.Equal(t, state.ApplyStateError, statuses["path/bad"].State)
	_, ok := s.Get("req-1")
	assert.True(t, ok)
	assert.Len(t, s.List(), 1)
	select {
	case <-s.Changes():
	default:
		t.Fatal("expected a change notification")
	}

	// Each update carries the full set, so an empty one removes everything.
	s.OnRCUpdate(map[string]state.RawConfig{}, ack)
	_, ok = s.Get("req-1")
	assert.False(t, ok)
}

func TestStoreFileAndRCPrecedence(t *testing.T) {
	s := NewStore(testCluster)
	fileReq := rawRequest(t, func(m map[string]any) { m["action"] = "revert" })
	require.NoError(t, s.setFileRequests([]byte("["+string(fileReq)+"]")))

	r, ok := s.Get("req-1")
	require.True(t, ok)
	assert.Equal(t, ActionRevert, r.Action)

	s.OnRCUpdate(map[string]state.RawConfig{"p": {Config: rawRequest(t, nil)}}, func(string, state.ApplyStatus) {})
	r, _ = s.Get("req-1")
	assert.Equal(t, ActionEnforce, r.Action, "Remote Config wins over the requests file")
	assert.Len(t, s.List(), 1)
}

func TestStoreFileBadJSONKeepsPrevious(t *testing.T) {
	s := NewStore(testCluster)
	require.NoError(t, s.setFileRequests([]byte("["+string(rawRequest(t, nil))+"]")))

	assert.Error(t, s.setFileRequests([]byte(`[{"id": "req-1",`)))
	_, ok := s.Get("req-1")
	assert.True(t, ok, "a bad file keeps the previous requests")
}

func TestStoreFileSkipsInvalidEntries(t *testing.T) {
	s := NewStore(testCluster)
	bad := rawRequest(t, func(m map[string]any) { m["id"] = "req-2"; m["control"] = "privileged" })
	require.NoError(t, s.setFileRequests([]byte("["+string(rawRequest(t, nil))+","+string(bad)+"]")))
	assert.Len(t, s.List(), 1)
}

func TestStoreWatchFileLoadsOnceOnAlreadyCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.json")
	require.NoError(t, os.WriteFile(path, []byte("["+string(rawRequest(t, nil))+"]"), 0o600))

	s := NewStore(testCluster)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.WatchFile(ctx, path, time.Hour)

	_, ok := s.Get("req-1")
	assert.True(t, ok, "the file is loaded once before the context is checked")
}

func TestStoreWatchFileMissingPathOnAlreadyCancelledContext(t *testing.T) {
	s := NewStore(testCluster)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.NotPanics(t, func() {
		s.WatchFile(ctx, filepath.Join(t.TempDir(), "missing.json"), time.Hour)
	})
	assert.Empty(t, s.List())
}

func TestParseIDs(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, ParseIDs(" a,,b ,"))
	assert.Empty(t, ParseIDs(""))
}
