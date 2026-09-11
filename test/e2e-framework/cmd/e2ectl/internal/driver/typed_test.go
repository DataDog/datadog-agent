// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package driver

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

type fakeParams struct {
	Count int `yaml:"count" default:"2" minimum:"0"`
	Other int `yaml:"other" default:"1" minimum:"0"`
}

type plainDriver struct {
	starts   int
	received fakeParams
}

func (*plainDriver) ID() string          { return "fake" }
func (*plainDriver) Description() string { return "Offline test driver." }
func (*plainDriver) Installers() []installer.Installer {
	return []installer.Installer{&installer.HostScript{}}
}
func (d *plainDriver) Start(p fakeParams, _ *config.File, _ envstore.Entry, _ *envstore.Store) error {
	d.starts++
	d.received = p
	return nil
}
func (*plainDriver) Stop(fakeParams, *config.File, envstore.Entry, *envstore.Store) error { return nil }

type checkedDriver struct {
	plainDriver
	calls int
}

func (d *checkedDriver) Validate(p fakeParams) error {
	d.calls++
	if p.Count < p.Other {
		return fmt.Errorf("count and other are incompatible")
	}
	return nil
}

func testFile(raw string) *config.File {
	return &config.File{Environment: config.Environment{Base: "fake", Section: []byte(raw)}}
}

func TestTypedDriverNeedsNoValidatorOrTemplate(t *testing.T) {
	backend := &plainDriver{}
	d := Define(configschema.Must[fakeParams](), "script", backend)
	cfg := testFile("")
	prepared, err := d.Prepare(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Environment.Section = []byte("count: 999") // cannot change already prepared execution
	if err := prepared.Start(envstore.Entry{}, nil); err != nil {
		t.Fatal(err)
	}
	if backend.received.Count != 2 || backend.received.Other != 1 || backend.starts != 1 {
		t.Fatalf("execution did not get typed defaults: %+v", backend)
	}
	if _, err := d.StarterConfig(); err != nil {
		t.Fatal(err)
	}
}

func TestOptionalDriverValidate(t *testing.T) {
	backend := &checkedDriver{}
	d := Define(configschema.Must[fakeParams](), "script", backend)
	if _, err := d.Prepare(testFile("count: -1")); err == nil || backend.calls != 0 {
		t.Fatal("automatic failures must not invoke the hook")
	}
	if _, err := d.Prepare(testFile("count: 0")); err == nil || !strings.Contains(err.Error(), "incompatible") || backend.calls != 1 {
		t.Fatalf("semantic hook missing: %v", err)
	}
	prepared, err := d.Prepare(testFile("count: 3"))
	if err != nil {
		t.Fatal(err)
	}
	if backend.calls != 2 {
		t.Fatalf("unexpected validation count: %d", backend.calls)
	}
	if err := prepared.Start(envstore.Entry{}, nil); err != nil {
		t.Fatal(err)
	}
	if backend.calls != 2 || backend.received.Count != 3 {
		t.Fatal("Start revalidated or re-decoded instead of using prepared input")
	}
	if _, err := d.StarterConfig(); err != nil {
		t.Fatal(err)
	}
	if backend.calls != 3 {
		t.Fatal("example generation must also invoke an existing hook")
	}
}

func TestPreparePreservesOriginalLocations(t *testing.T) {
	cfg, errs := config.Parse([]byte("schema: 1\nenvironment:\n  kind:\n    nodes: -1\n  base: kind\nagent:\n  install: helm\n"))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	cfg.Path = "input.yaml"
	d, err := Get("kind")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Prepare(cfg); err == nil || !strings.Contains(err.Error(), "input.yaml: environment.kind.nodes (line 4") {
		t.Fatalf("field location was lost: %v", err)
	}
}
