// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package buildprovider

import (
	"context"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
)

func TestRegistriesRejectIncompatibleProvidersAndUnknownFields(t *testing.T) {
	s := &config.BuildSelection{Provider: "existing-package", Section: []byte("path: /tmp/agent.deb")}
	if err := Packages.Validate(s); err != nil {
		t.Fatal(err)
	}
	if Images.Validate(s) == nil || Binaries.Validate(s) == nil {
		t.Fatal("accepted incompatible format")
	}
	s.Section = []byte("path: /tmp/agent.rpm")
	if Packages.Validate(s) == nil {
		t.Fatal("accepted untested RPM")
	}
	s = &config.BuildSelection{Provider: "existing-image", Section: []byte("reference: localhost/agent:dev\nrace: true\n")}
	if Images.Validate(s) == nil {
		t.Fatal("foreign provider field accepted")
	}
}
func TestTypedRegistryValidationDoesNotPrepare(t *testing.T) {
	type Params struct {
		Path string `yaml:"path" config:"required"`
	}
	called := false
	definition := Define("fake", "offline typed provider", configschema.Must[Params](), func(p Params) error {
		if p.Path != "ok" {
			t.Fatal(p)
		}
		return nil
	}, func(context.Context, Params, Request) (ImageResult, error) { called = true; return ImageResult{}, nil })
	r := NewRegistry(definition)
	if err := r.Validate(&config.BuildSelection{Provider: "fake", Section: []byte("path: ok")}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("validation prepared artifact")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration accepted")
		}
	}()
	NewRegistry(definition, definition)
}
func TestExistingImagesNeverInvokeBuilderOnRepeatedPreparation(t *testing.T) {
	calls := 0
	a := agentbuild.Adapter{Run: func(_ context.Context, i agentbuild.Invocation) ([]byte, error) {
		calls++
		if i.Program != "docker" || strings.Join(i.Args, " ") != "image inspect localhost/agent:dev" {
			t.Fatal(i)
		}
		return []byte(`[{"Id":"sha256:` + strings.Repeat("a", 64) + `","Os":"linux","Architecture":"amd64"}]`), nil
	}}
	s := &config.BuildSelection{Provider: "existing-image", Section: []byte("reference: localhost/agent:dev")}
	for range 2 {
		if _, err := Images.Prepare(context.Background(), s, Request{Adapter: a, Target: agentbuild.Target{OS: "linux", Arch: "amd64"}}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	if Images.ValidateRouting(s) == nil {
		t.Fatal("unattested image accepted explicit receiver")
	}
}
