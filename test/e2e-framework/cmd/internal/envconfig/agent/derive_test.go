// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strings"
	"testing"

	binaryconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/binary"
	helmconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/helm"
	pc "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/localpackage"
	scriptconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/script"
	"go.yaml.in/yaml/v3"
)

// The derivation table as implemented: every supported cell resolves to the
// documented installer/build pair; every unsupported cell rejects with the
// documented message.
func TestDeriveTable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		base      string
		cfg       Config
		installer string
		provider  string // "" = no build selection
		section   string
	}{
		{"source on local", "local", Config{Source: true}, "binary", "invoke-binary", ""},
		{"source on kind", "kind", Config{Source: true}, "helm", "invoke-image", ""},
		{"pipeline on local", "local", Config{Pipeline: 138372337}, "package", "pipeline", ""},
		{"pipeline on ec2-host", "ec2-host", Config{Pipeline: 138372337}, "package", "pipeline", ""},
		{"pipeline on docker-host", "docker-host", Config{Pipeline: 138372337}, "package", "pipeline", ""},
		{"version on kind", "kind", Config{Version: "7.83.0"}, "helm", "", "version: 7.83.0"},
		{"version on eks", "eks", Config{Version: "7.83.0"}, "helm", "", "version: 7.83.0"},
		{"version on ec2-host", "ec2-host", Config{Version: "7.69.0"}, "script", "", "version: 7.69.0"},
		{"version on docker-host", "docker-host", Config{Version: "7.69.0"}, "script", "", "version: 7.69.0"},
		{"default on local", "local", Config{}, "binary", "invoke-binary", ""},
		{"default on kind", "kind", Config{}, "helm", "", "version: " + DefaultVersion},
		{"default on eks", "eks", Config{}, "helm", "", "version: " + DefaultVersion},
		{"default on ec2-host", "ec2-host", Config{}, "script", "", "version: " + DefaultVersion},
		{"default on docker-host", "docker-host", Config{}, "script", "", "version: " + DefaultVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Derive(tc.base, tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if res.Installer != tc.installer {
				t.Fatalf("installer %q, want %q", res.Installer, tc.installer)
			}
			if tc.provider == "" && res.Build != nil {
				t.Fatalf("unexpected build provider %q", res.Build.Provider)
			}
			if tc.provider != "" && (res.Build == nil || res.Build.Provider != tc.provider) {
				t.Fatalf("build provider %v, want %q", res.Build, tc.provider)
			}
			if tc.section != "" && !strings.Contains(string(res.Section), tc.section) {
				t.Fatalf("section %q does not contain %q", res.Section, tc.section)
			}
		})
	}
}

func TestDeriveRejections(t *testing.T) {
	for _, tc := range []struct {
		base string
		cfg  Config
		want string
	}{
		{"ec2-host", Config{Source: true}, "local source builds are not yet automated for remote targets"},
		{"docker-host", Config{Source: true}, "local source builds are not yet automated for remote targets"},
		{"eks", Config{Source: true}, "local image builds cannot be delivered to remote clusters yet"},
		{"kind", Config{Pipeline: 1}, "pipeline images are not downloadable yet"},
		{"eks", Config{Pipeline: 1}, "pipeline images are not downloadable yet — use version for a released chart"},
		{"local", Config{Version: "7.83.0"}, "released versions install on the Helm bases"},
		{"local", Config{Source: true, Version: "7.83.0"}, "mutually exclusive"},
		{"local", Config{Values: "datadog: {}\n"}, "Helm chart values are supported on the Helm bases kind and eks only"},
		{"ec2-host", Config{Values: "datadog: {}\n"}, "Helm chart values are supported on the Helm bases kind and eks only"},
		{"local", Config{Pipeline: 1, Integrations: map[string]string{"cpu.d": "x"}}, "the container target runs the fixed core checks"},
		{"kind", Config{Version: "7.83.0", Config: "log_level: debug"}, "no Helm chart location"},
		{"unknown", Config{Source: true}, "does not support source builds"},
		{"unknown", Config{}, "has no default source"},
	} {
		t.Run(tc.base+" "+tc.want, func(t *testing.T) {
			_, err := Derive(tc.base, tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got: %v", tc.want, err)
			}
		})
	}
}

// The resolved sections must decode against their installer schemas: the
// derivation feeds the existing machinery, so its output must satisfy their
// contracts.
func TestDerivedSectionsDecode(t *testing.T) {
	// binary section
	res, err := Derive("local", Config{Source: true, Config: "log_level: debug", Integrations: map[string]string{"cpu.d": "init_config: {}\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := binaryconfig.Schema.Decode(res.Section, "agent.binary"); err != nil {
		t.Fatalf("derived binary section invalid: %v", err)
	}
	// script section
	res, err = Derive("ec2-host", Config{Version: "7.83.0", Config: "log_level: debug"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := scriptconfig.Schema.Decode(res.Section, "agent.script"); err != nil {
		t.Fatalf("derived script section invalid: %v", err)
	}
	// package section (pipeline)
	res, err = Derive("local", Config{Pipeline: 138372337, Config: "log_level: debug"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pc.Schema.Decode(res.Section, "agent.package"); err != nil {
		t.Fatalf("derived package section invalid: %v", err)
	}
	// helm section (version)
	res, err = Derive("kind", Config{Version: "7.83.0", Values: "datadog:\n  logLevel: DEBUG\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := helmconfig.Schema.Decode(res.Section, "agent.helm"); err != nil {
		t.Fatalf("derived helm section invalid: %v", err)
	}
	// helm section (version on the remote Kubernetes base)
	res, err = Derive("eks", Config{Version: "7.83.0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := helmconfig.Schema.Decode(res.Section, "agent.helm"); err != nil {
		t.Fatalf("derived eks helm section invalid: %v", err)
	}
	// script section (version on the docker host base)
	res, err = Derive("docker-host", Config{Version: "7.83.0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := scriptconfig.Schema.Decode(res.Section, "agent.script"); err != nil {
		t.Fatalf("derived docker-host script section invalid: %v", err)
	}
}

// Integrations on kind route into the chart's datadog.confd; user values are
// preserved and merged, and integration keys win only per check name.
func TestKindIntegrationsRouteToConfd(t *testing.T) {
	res, err := Derive("kind", Config{
		Version:      "7.83.0",
		Values:       "datadog:\n  logLevel: DEBUG\n",
		Integrations: map[string]string{"cpu.d": "init_config: {}\ninstances:\n- {}\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var section map[string]any
	if err := yaml.Unmarshal(res.Section, &section); err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := yaml.Unmarshal([]byte(section["values"].(string)), &values); err != nil {
		t.Fatal(err)
	}
	datadog := values["datadog"].(map[string]any)
	if datadog["logLevel"] != "DEBUG" {
		t.Fatalf("user values were not preserved: %v", section)
	}
	confd := datadog["confd"].(map[string]any)
	if confd["cpu"] != "init_config: {}\ninstances:\n- {}\n" {
		t.Fatalf("integrations were not routed to datadog.confd: %v", confd)
	}
}

func TestDefaultSourceAndExample(t *testing.T) {
	if DefaultSource("local") != "source: true" || DefaultSource("kind") != "version: "+DefaultVersion {
		t.Fatal("unexpected default sources")
	}
	for _, base := range []string{"local", "kind", "eks", "ec2-host", "docker-host"} {
		node, err := Example(base)
		if err != nil {
			t.Fatal(err)
		}
		data, err := yaml.Marshal(node)
		if err != nil {
			t.Fatal(err)
		}
		// The starter selection must select the base default and explain the
		// three options in its comments.
		if !strings.Contains(string(data), DefaultSourceField(base)+":") {
			t.Fatalf("starter for %q does not select the default source: %s", base, data)
		}
		if len(node.Content) < 2 || !strings.Contains(node.Content[0].HeadComment, "pick exactly ONE") {
			t.Fatalf("starter for %q does not explain the source options", base)
		}
	}
}
