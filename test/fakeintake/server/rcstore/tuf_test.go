// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcstore

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalJSON_SortsKeys(t *testing.T) {
	v := map[string]any{"b": 2, "a": 1, "c": map[string]any{"y": 1, "x": 2}}
	got, err := CanonicalJSON(v)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":1,"b":2,"c":{"x":2,"y":1}}`
	if string(got) != want {
		t.Fatalf("canonical JSON mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestCanonicalJSON_NoHTMLEscape(t *testing.T) {
	v := map[string]any{"k": "<a>&</a>"}
	got, err := CanonicalJSON(v)
	if err != nil {
		t.Fatal(err)
	}
	// Default Go encoding HTML-escapes < > & to <, >, &.
	// We require the literal characters so go-tuf computes a matching keyid.
	if !strings.Contains(string(got), "<a>&</a>") {
		t.Fatalf("expected literal <a>&</a>, got %s", got)
	}
	if strings.Contains(string(got), `\u003c`) || strings.Contains(string(got), `\u0026`) {
		t.Fatalf("did not expect HTML escape sequences, got %s", got)
	}
}

func TestSignEnvelope_RoundTrip(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := PublicKeyHex(priv)
	keyID, err := ComputeKeyID(pubHex)
	if err != nil {
		t.Fatal(err)
	}

	env, err := SignEnvelope(priv, keyID, map[string]any{"hello": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyEnvelope(priv.Public().(ed25519.PublicKey), env); err != nil {
		t.Fatalf("verify: %v", err)
	}

	var parsed struct {
		Signatures []map[string]string `json:"signatures"`
		Signed     json.RawMessage     `json:"signed"`
	}
	if err := json.Unmarshal(env, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Signatures[0]["keyid"] != keyID {
		t.Fatalf("keyid mismatch: %s vs %s", parsed.Signatures[0]["keyid"], keyID)
	}
}

func TestComputeKeyID_StableLength(t *testing.T) {
	id, err := ComputeKeyID("00")
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 64 {
		t.Fatalf("expected 64 hex chars, got %d (%s)", len(id), id)
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("not hex: %v", err)
	}
}

func TestGenerateTUFMetas_Verifies(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	pubHex := PublicKeyHex(priv)
	keyID, err := ComputeKeyID(pubHex)
	if err != nil {
		t.Fatal(err)
	}
	root, err := BuildRootJSON(priv, keyID, pubHex)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyEnvelope(pub, root); err != nil {
		t.Fatalf("verify root: %v", err)
	}

	cfgs := []Config{
		{OrgID: "42", Product: "METRIC_CONTROL", ConfigID: "abc", ConfigName: "filterlist", Data: []byte(`{"k":"v"}`)},
	}
	metas, err := GenerateTUFMetas(cfgs, priv, keyID, root, 5)
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"targets": metas.Targets, "snapshot": metas.Snapshot, "timestamp": metas.Timestamp} {
		if err := VerifyEnvelope(pub, b); err != nil {
			t.Fatalf("verify %s: %v", name, err)
		}
	}
}

func TestConfigPath(t *testing.T) {
	c := Config{OrgID: "42", Product: "METRIC_CONTROL", ConfigID: "abc", ConfigName: "filterlist"}
	if got, want := c.Path(), "datadog/42/METRIC_CONTROL/abc/filterlist"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
