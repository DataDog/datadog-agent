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

const (
	testCluster = "cluster-1"
	testDigest  = "33720bb7ebaf56d8cfc80a99b420c230cd5625d7383813057431170a4033662a"
	testImageID = "registry.example.com/shop/web@sha256:" + testDigest
)

// rawRequest returns a valid request as JSON, after mutate edits its fields.
func rawRequest(t *testing.T, mutate func(m map[string]any)) []byte {
	m := map[string]any{
		"id":      "req-1",
		"action":  "trial",
		"control": "capabilities",
		"k8s_target": map[string]any{
			"cluster": testCluster, "kind": "Deployment", "namespace": "shop",
			"name": "web", "container": "app", "image_id": testImageID,
		},
		"parameters": map[string]any{"capabilities_add": []string{"cap_net_bind_service"}},
		"created_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
	}
	if mutate != nil {
		mutate(m)
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

func target(m map[string]any) map[string]any { return m["k8s_target"].(map[string]any) }

func TestParseRequest(t *testing.T) {
	r, err := parseRequest(rawRequest(t, nil), testCluster)
	require.NoError(t, err)
	assert.Equal(t, "req-1", r.ID)
	assert.Equal(t, []string{"NET_BIND_SERVICE"}, r.Parameters.CapabilitiesAdd)
	assert.NotEmpty(t, r.hash)

	invalid := map[string]func(m map[string]any){
		"unknown field":        func(m map[string]any) { m["patch"] = "{}" },
		"id with comma":        func(m map[string]any) { m["id"] = "a,b" },
		"empty id":             func(m map[string]any) { m["id"] = "" },
		"unknown action":       func(m map[string]any) { m["action"] = "apply" },
		"unknown control":      func(m map[string]any) { m["control"] = "privileged" },
		"capabilities on ro":   func(m map[string]any) { m["control"] = "read_only_root_fs" },
		"capability ALL":       func(m map[string]any) { m["parameters"] = map[string]any{"capabilities_add": []string{"ALL"}} },
		"malformed capability": func(m map[string]any) { m["parameters"] = map[string]any{"capabilities_add": []string{"NET BIND"}} },
		"unknown capability":   func(m map[string]any) { m["parameters"] = map[string]any{"capabilities_add": []string{"FOO"}} },
		"statefulset":          func(m map[string]any) { target(m)["kind"] = "StatefulSet" },
		"uid":                  func(m map[string]any) { target(m)["uid"] = "uid-1" },
		"missing container":    func(m map[string]any) { delete(target(m), "container") },
		"missing image_id":     func(m map[string]any) { delete(target(m), "image_id") },
		"image_id by tag":      func(m map[string]any) { target(m)["image_id"] = "registry.example.com/shop/web:1.2" },
		"image_id short":       func(m map[string]any) { target(m)["image_id"] = "web@sha256:abc" },
		"empty cluster":        func(m map[string]any) { target(m)["cluster"] = "" },
		"other cluster":        func(m map[string]any) { target(m)["cluster"] = "cluster-2" },
		"missing created_at":   func(m map[string]any) { delete(m, "created_at") },
		"missing expires_at":   func(m map[string]any) { delete(m, "expires_at") },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := parseRequest(rawRequest(t, mutate), testCluster)
			assert.Error(t, err)
		})
	}
}

func TestParseRequestValidate(t *testing.T) {
	r, err := parseRequest(rawRequest(t, func(m map[string]any) { m["action"] = "validate" }), testCluster)
	require.NoError(t, err)
	assert.False(t, r.Active(time.Now()), "a validate request is never applied")
}

// TestParseRequestRFCExample parses Appendix G's H1 example verbatim.
func TestParseRequestRFCExample(t *testing.T) {
	raw := `{
		"id": "hr-01JBA7K3Q2M8N4P6R9T1V5X7Z0",
		"action": "trial",
		"control": "capabilities",
		"k8s_target": {
			"cluster": "3b6f2c1e-9d4a-4f7e-8c25-1a0e7d9b5f42",
			"kind": "Deployment",
			"namespace": "payments",
			"name": "checkout-api",
			"container": "api",
			"image_id": "registry.example.com/payments/checkout-api@sha256:33720bb7ebaf56d8cfc80a99b420c230cd5625d7383813057431170a4033662a"
		},
		"parameters": {
			"capabilities_add": ["NET_BIND_SERVICE"]
		},
		"created_at": "2026-10-21T15:00:00Z",
		"expires_at": "2026-10-28T15:00:00Z"
	}`
	r, err := parseRequest([]byte(raw), "3b6f2c1e-9d4a-4f7e-8c25-1a0e7d9b5f42")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC), r.CreatedAt)
	assert.Equal(t, "33720bb7ebaf56d8cfc80a99b420c230cd5625d7383813057431170a4033662a", Digest(r.Target.ImageID))
}

func TestDigest(t *testing.T) {
	assert.Equal(t, testDigest, Digest(testImageID))
	assert.Equal(t, testDigest, Digest("docker-pullable://web@sha256:"+testDigest), "a Docker imageID")
	assert.Equal(t, testDigest, Digest("web:1.2@sha256:"+testDigest), "a tag and a digest")
	assert.Empty(t, Digest("sha256:"+testDigest), "an image config id is not a registry digest")
	assert.Empty(t, Digest("web:1.2"))
}

// TestParseRequestEmptyClusterNeverMatches guards against an unconfigured
// (empty) Cluster Agent clusterID accidentally matching a request whose
// k8s_target.cluster is also empty.
func TestParseRequestEmptyClusterNeverMatches(t *testing.T) {
	raw := rawRequest(t, func(m map[string]any) { target(m)["cluster"] = "" })
	_, err := parseRequest(raw, "")
	assert.Error(t, err)
}

func TestRequestActive(t *testing.T) {
	now := time.Now()
	assert.True(t, (&Request{Action: ActionTrial, ExpiresAt: now.Add(time.Minute)}).Active(now))
	assert.False(t, (&Request{Action: ActionTrial, ExpiresAt: now.Add(-time.Minute)}).Active(now))
	assert.False(t, (&Request{Action: ActionRevert, ExpiresAt: now.Add(time.Minute)}).Active(now))
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
	assert.Equal(t, ActionTrial, r.Action, "Remote Config wins over the requests file")
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
