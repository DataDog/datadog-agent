// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfig

import (
	"strings"
	"testing"
)

func TestGenerateWiresFakeintake(t *testing.T) {
	cfg, err := Generate("test-key", &Endpoint{Scheme: "http", Host: "fakeintake.internal", Port: 8080}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`api_key: "test-key"`,
		"dd_url: http://fakeintake.internal:8080",
		"logs_config.logs_dd_url: fakeintake.internal:8080",
		"logs_config.logs_no_ssl: true",
		"logs_config.force_use_http: true",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("missing %q in generated config:\n%s", want, cfg)
		}
	}
}

func TestGenerateWithoutEndpoint(t *testing.T) {
	cfg, err := Generate("test-key", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cfg, "dd_url") {
		t.Fatalf("no endpoint must mean no wiring, got:\n%s", cfg)
	}
}

func TestGenerateMergesExtraConfig(t *testing.T) {
	cfg, err := Generate("test-key", &Endpoint{Scheme: "http", Host: "fakeintake.internal", Port: 8080}, "log_level: debug\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg, "log_level: debug") || !strings.Contains(cfg, "dd_url: http://fakeintake.internal:8080") {
		t.Fatalf("generated wiring and user config must coexist:\n%s", cfg)
	}
}

func TestGenerateRejectsInvalidExtraConfig(t *testing.T) {
	if _, err := Generate("test-key", nil, "not: [valid"); err == nil {
		t.Fatal("invalid user YAML must fail generation")
	}
}
