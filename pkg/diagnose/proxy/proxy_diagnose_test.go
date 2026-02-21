/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- helpers ---

func useEmptyConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(""), 0644); err != nil {
		t.Fatalf("write empty datadog.yaml: %v", err)
	}
	t.Setenv("DD_CONF_DIR", dir)
}

func hasFinding(code string, findings []Finding) bool {
	for _, f := range findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

// --- tests ---

func TestRun_JSON_EndpointMatrixPresentEvenWhenEmpty(t *testing.T) {
	useEmptyConfig(t)
	t.Setenv("DD_PROXY_HTTP", "")
	t.Setenv("DD_PROXY_HTTPS", "")
	t.Setenv("DD_NO_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("NO_PROXY", "")

	res := Run(true)

	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("json.Marshal(Result): %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("json.Unmarshal(Result): %v", err)
	}
	v, ok := m["endpoint_matrix"]
	if !ok {
		t.Fatalf("endpoint_matrix key missing in JSON: %s", string(raw))
	}
	if _, ok := v.([]any); !ok {
		t.Fatalf("endpoint_matrix is not a JSON array (got %T): %v", v, v)
	}
}

func TestComputeEffective_Precedence_StdEnvThenDDEnv(t *testing.T) {
	useEmptyConfig(t)

	// std env only → chosen with SourceStdEnv
	t.Setenv("HTTPS_PROXY", "http://std:8443")
	t.Setenv("DD_PROXY_HTTPS", "")
	eff := ComputeEffective()
	if eff.HTTPS.Value != "http://std:8443" || eff.HTTPS.Source != SourceStdEnv {
		t.Fatalf("std_env precedence failed: got %+v", eff.HTTPS)
	}

	// dd env overrides std env
	t.Setenv("DD_PROXY_HTTPS", "http://dd:443")
	eff = ComputeEffective()
	if eff.HTTPS.Value != "http://dd:443" || eff.HTTPS.Source != SourceDDEnv {
		t.Fatalf("dd_env precedence failed: got %+v", eff.HTTPS)
	}
}

func TestRun_Lints_UnknownScheme(t *testing.T) {
	useEmptyConfig(t)
	t.Setenv("DD_PROXY_HTTPS", "socks5://corp-proxy:1080")

	res := Run(true) // config-only
	if !hasFinding("proxy.https.unknown_scheme", res.Findings) {
		raw, _ := json.MarshalIndent(res, "", "  ")
		t.Fatalf("expected finding proxy.https.unknown_scheme; got:\n%s", string(raw))
	}
	if res.Summary != SeverityYellow && res.Summary != SeverityRed {
		t.Fatalf("expected summary yellow/red due to lint; got %s", res.Summary)
	}
}

func TestRun_Lints_ConflictDDvsStd(t *testing.T) {
	useEmptyConfig(t)
	t.Setenv("DD_PROXY_HTTPS", "http://dd:443")
	t.Setenv("HTTPS_PROXY", "http://std:8443")

	res := Run(true)
	if !hasFinding("proxy.https.conflict", res.Findings) {
		raw, _ := json.MarshalIndent(res, "", "  ")
		t.Fatalf("expected finding proxy.https.conflict; got:\n%s", string(raw))
	}
}
