// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package driver

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
)

func TestRegistryInvariants(t *testing.T) {
	if len(IDs()) == 0 {
		t.Fatal("no drivers registered")
	}
	seen := make(map[string]bool)
	for _, id := range IDs() {
		if seen[id] {
			t.Fatalf("duplicate driver ID %q would hide an environment from discovery", id)
		}
		seen[id] = true
		d, err := Get(id)
		if err != nil {
			t.Fatalf("Get(%q): %v", id, err)
		}
		if d.ID() != id {
			t.Errorf("driver ID mismatch: %q != %q", d.ID(), id)
		}
		if len(d.Installers()) == 0 {
			t.Errorf("driver %q advertises no installers", id)
		}
		for _, i := range d.Installers() {
			if i.ID() == "" {
				t.Errorf("driver %q has an installer with an empty ID", id)
			}
		}
	}
}

func TestGetUnknownBaseListsRegistered(t *testing.T) {
	_, err := Get("nope")
	if err == nil || !strings.Contains(err.Error(), "registered:") {
		t.Fatalf("expected an error listing the registered bases, got: %v", err)
	}
}

func TestInstallerForListsSupported(t *testing.T) {
	d, err := Get("kind")
	if err != nil {
		t.Fatal(err)
	}
	_, err = InstallerFor(d, "script")
	if err == nil || !strings.Contains(err.Error(), `not supported for base "kind" (supported: helm)`) {
		t.Fatalf("expected an unsupported-installer error listing the supported ones, got: %v", err)
	}
}

func TestDriverSectionValidation(t *testing.T) {
	// kind: bad version inside the driver-owned section
	badKind := `
schema: 1
environment:
  base: kind
  kind:
    version: "1.31"
agent:
  install: helm
`
	f, errs := config.Parse([]byte(badKind))
	if len(errs) > 0 {
		t.Fatalf("generic validation should pass, got: %v", errs)
	}
	d, err := Get(f.Environment.Base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Prepare(f); err == nil || !strings.Contains(err.Error(), "environment.kind.version") {
		t.Fatalf("expected a field-anchored kind section error, got: %v", err)
	}

	// ec2-host: missing section
	badEC2 := []byte(`
schema: 1
environment:
  base: ec2-host
agent:
  install: script
`)
	f, errs = config.Parse(badEC2)
	if len(errs) > 0 {
		t.Fatalf("generic validation should pass, got: %v", errs)
	}
	d, _ = Get(f.Environment.Base)
	if _, err := d.Prepare(f); err == nil || !strings.Contains(err.Error(), "environment.ec2-host") {
		t.Fatalf("expected a field-anchored ec2-host section error, got: %v", err)
	}
}
