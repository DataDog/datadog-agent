// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	core "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/server/rcstore"
)

func newRCTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), "signing.key")
	ready := make(chan bool, 1)
	fi := NewServer(
		WithReadyChannel(ready),
		WithRemoteConfig("42"),
		WithRemoteConfigKeyPath(keyPath),
	)
	if fi.rc == nil {
		t.Fatal("rc not enabled")
	}
	ts := httptest.NewServer(fi.server.Handler)
	t.Cleanup(ts.Close)
	return ts, fi
}

func TestRCAddListAndDelete(t *testing.T) {
	ts, fi := newRCTestServer(t)

	body, _ := json.Marshal(api.RCAddConfigRequest{
		Product: "METRIC_CONTROL", ConfigID: "abc", ConfigName: "fl",
		Data: json.RawMessage(`{"blocked_metrics":{}}`),
	})
	resp, err := http.Post(ts.URL+"/fakeintake/rc/config", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add: status %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/fakeintake/rc/configs")
	if err != nil {
		t.Fatal(err)
	}
	var got []api.RCConfig
	_ = json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if len(got) != 1 || got[0].ConfigID != "abc" {
		t.Fatalf("list: %+v", got)
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/fakeintake/rc/config/42/METRIC_CONTROL/abc/fl", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d", resp.StatusCode)
	}
	if got := fi.rc.snapshot(); len(got) != 0 {
		t.Fatalf("expected empty after delete, got %+v", got)
	}
}

func TestRCConfigurationsServesSignedMetas(t *testing.T) {
	ts, fi := newRCTestServer(t)

	fi.rc.addConfig(rcstore.Config{
		OrgID: "42", Product: "METRIC_CONTROL",
		ConfigID: "abc", ConfigName: "fl",
		Data: []byte(`{"k":"v"}`),
	})

	reqProto := &core.LatestConfigsRequest{
		Hostname:     "host",
		AgentVersion: "test",
		Products:     []string{"METRIC_CONTROL"},
	}
	body, err := proto.Marshal(reqProto)
	if err != nil {
		t.Fatal(err)
	}
	httpReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v0.1/configurations", bytes.NewReader(body))
	httpReq.Header.Set("DD-Api-Key", "test-api-key")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	respBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	out := &core.LatestConfigsResponse{}
	if err := proto.Unmarshal(respBytes, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.GetTargetFiles()) != 1 {
		t.Fatalf("expected 1 target file, got %d", len(out.GetTargetFiles()))
	}
	if out.GetTargetFiles()[0].Path != "datadog/42/METRIC_CONTROL/abc/fl" {
		t.Fatalf("bad path: %s", out.GetTargetFiles()[0].Path)
	}

	pub := fi.rc.signing.Public().(ed25519.PublicKey)
	for name, top := range map[string]*core.TopMeta{
		"root":      out.GetConfigMetas().GetRoots()[0],
		"timestamp": out.GetConfigMetas().GetTimestamp(),
		"snapshot":  out.GetConfigMetas().GetSnapshot(),
		"targets":   out.GetConfigMetas().GetTopTargets(),
	} {
		if err := rcstore.VerifyEnvelope(pub, top.Raw); err != nil {
			t.Fatalf("verify %s: %v", name, err)
		}
	}
}

func TestRCConfigurationsServesSignedMetadataWithoutConfigs(t *testing.T) {
	ts, fi := newRCTestServer(t)

	body, err := proto.Marshal(&core.LatestConfigsRequest{
		Hostname: "host", AgentVersion: "test", Products: []string{"AP_RUNNER_KEYS"},
	})
	if err != nil {
		t.Fatal(err)
	}
	httpReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v0.1/configurations", bytes.NewReader(body))
	httpReq.Header.Set("DD-Api-Key", "test-api-key")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	respBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	out := &core.LatestConfigsResponse{}
	if err := proto.Unmarshal(respBytes, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.GetTargetFiles()) != 0 {
		t.Fatalf("expected no target files, got %d", len(out.GetTargetFiles()))
	}

	pub := fi.rc.signing.Public().(ed25519.PublicKey)
	for name, top := range map[string]*core.TopMeta{
		"root":      out.GetConfigMetas().GetRoots()[0],
		"timestamp": out.GetConfigMetas().GetTimestamp(),
		"snapshot":  out.GetConfigMetas().GetSnapshot(),
		"targets":   out.GetConfigMetas().GetTopTargets(),
	} {
		if err := rcstore.VerifyEnvelope(pub, top.Raw); err != nil {
			t.Fatalf("verify %s: %v", name, err)
		}
	}
}

func TestRCConfigurationsRecordsApplyStates(t *testing.T) {
	ts, _ := newRCTestServer(t)

	body, err := proto.Marshal(&core.LatestConfigsRequest{
		ActiveClients: []*core.Client{{
			Id: "par-client",
			State: &core.ClientState{ConfigStates: []*core.ConfigState{{
				Id: "fake-runner-key", Product: "AP_RUNNER_KEYS", Version: 2, ApplyState: 2,
			}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(ts.URL+"/api/v0.1/configurations", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/fakeintake/rc/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var stats api.RCStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if len(stats.ApplyStates) != 1 || stats.ApplyStates[0].ConfigID != "fake-runner-key" || stats.ApplyStates[0].ApplyState != 2 {
		t.Fatalf("unexpected apply states: %+v", stats.ApplyStates)
	}
}

func TestRCConfigurationsIgnoresRequestedProducts(t *testing.T) {
	ts, fi := newRCTestServer(t)

	fi.rc.addConfig(rcstore.Config{
		OrgID: "42", Product: "AP_RUNNER_KEYS",
		ConfigID: "fake-runner-key", ConfigName: "key",
		Data: []byte(`{"k":"v"}`),
	})

	poll := func(products ...string) *core.LatestConfigsResponse {
		t.Helper()
		body, err := proto.Marshal(&core.LatestConfigsRequest{
			Hostname: "host", AgentVersion: "test", Products: products,
		})
		if err != nil {
			t.Fatal(err)
		}
		httpReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v0.1/configurations", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/x-protobuf")
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatal(err)
		}
		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", resp.StatusCode, respBytes)
		}
		out := &core.LatestConfigsResponse{}
		if err := proto.Unmarshal(respBytes, out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return out
	}

	first := poll("AGENT_CONFIG")
	second := poll("AGENT_CONFIG", "AP_RUNNER_KEYS")

	for name, out := range map[string]*core.LatestConfigsResponse{"first": first, "second": second} {
		if len(out.GetTargetFiles()) != 1 ||
			out.GetTargetFiles()[0].GetPath() != "datadog/42/AP_RUNNER_KEYS/fake-runner-key/key" {
			t.Fatalf("%s poll: unexpected target files %+v", name, out.GetTargetFiles())
		}
	}
	if v := second.GetDirectorMetas().GetTargets().GetVersion(); v != first.GetDirectorMetas().GetTargets().GetVersion() {
		t.Fatalf("targets version changed without a config change: %d then %d",
			first.GetDirectorMetas().GetTargets().GetVersion(), v)
	}
}

func TestRCSetExpirationChangesServedMetadata(t *testing.T) {
	ts, fi := newRCTestServer(t)

	poll := func() *core.LatestConfigsResponse {
		t.Helper()
		body, err := proto.Marshal(&core.LatestConfigsRequest{
			Hostname: "host", AgentVersion: "test", Products: []string{"AP_RUNNER_KEYS"},
		})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.Post(ts.URL+"/api/v0.1/configurations", "application/x-protobuf", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status %d: %s", resp.StatusCode, b)
		}
		respBytes, _ := io.ReadAll(resp.Body)
		out := &core.LatestConfigsResponse{}
		if err := proto.Unmarshal(respBytes, out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return out
	}

	expiresOf := func(raw []byte) string {
		t.Helper()
		var envelope struct {
			Signed struct {
				Expires string `json:"expires"`
			} `json:"signed"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		return envelope.Signed.Expires
	}

	before := poll()

	expiresAt := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	body, _ := json.Marshal(api.RCSetExpirationRequest{ExpiresAt: expiresAt})
	resp, err := http.Post(ts.URL+"/fakeintake/rc/expiration", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("set expiration: status %d", resp.StatusCode)
	}

	after := poll()

	const want = "2000-01-02T03:04:05Z"
	for name, raw := range map[string][]byte{
		"timestamp": after.GetConfigMetas().GetTimestamp().GetRaw(),
		"snapshot":  after.GetConfigMetas().GetSnapshot().GetRaw(),
		"targets":   after.GetConfigMetas().GetTopTargets().GetRaw(),
	} {
		if got := expiresOf(raw); got != want {
			t.Fatalf("%s expires = %q, want %q", name, got, want)
		}
	}

	// The version must advance so the Agent treats the shortened horizon as a
	// new revision rather than a cached one.
	beforeVersion := before.GetConfigMetas().GetTopTargets().GetVersion()
	afterVersion := after.GetConfigMetas().GetTopTargets().GetVersion()
	if afterVersion <= beforeVersion {
		t.Fatalf("targets version did not advance: %d then %d", beforeVersion, afterVersion)
	}

	// Metadata must stay verifiable against the signing key after the rewrite.
	pub := fi.rc.signing.Public().(ed25519.PublicKey)
	if err := rcstore.VerifyEnvelope(pub, after.GetConfigMetas().GetTopTargets().GetRaw()); err != nil {
		t.Fatalf("verify targets: %v", err)
	}
}

func TestRCSetExpirationRejectsMissingExpiry(t *testing.T) {
	ts, _ := newRCTestServer(t)

	resp, err := http.Post(ts.URL+"/fakeintake/rc/expiration", "application/json",
		bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRCDisabledReturns404(t *testing.T) {
	ready := make(chan bool, 1)
	fi := NewServer(WithReadyChannel(ready))
	ts := httptest.NewServer(fi.server.Handler)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/fakeintake/rc/configs")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestRCStats(t *testing.T) {
	ts, _ := newRCTestServer(t)
	resp, err := http.Get(ts.URL + "/fakeintake/rc/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var s api.RCStats
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if s.KeyID == "" || s.PublicKey == "" || s.RootJSON == "" {
		t.Fatalf("stats missing fields: %+v", s)
	}
}
