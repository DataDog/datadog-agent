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
