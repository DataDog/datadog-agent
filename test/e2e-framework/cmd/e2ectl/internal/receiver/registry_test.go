// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package receiver

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"testing"
)

func selection(typ, section string) *config.ReceiverSelection {
	return &config.ReceiverSelection{Type: typ, Section: []byte(section)}
}
func TestRegistrySchemaValidationIsOffline(t *testing.T) {
	for _, tc := range []struct {
		s     *config.ReceiverSelection
		valid bool
	}{
		{nil, true}, {selection("fakeintake", ""), true}, {selection("fakeintake", "remote-config: disabled"), true},
		{selection("fakeintake", "remote-config: receiver"), false}, {selection("blackhole", "url: http://sink:8080"), true},
		{selection("blackhole", ""), true}, {selection("blackhole", "{}"), true},
		{selection("datadog", "site: datadoghq.eu\napi-key-ref: runner/api_key"), true}, {selection("datadog", "site: datadoghq.eu\napi-key: secret"), false},
		{selection("other", ""), false}, {selection("blackhole", "url: ftp://sink"), false},
	} {
		if err := Validate(tc.s); (err == nil) != tc.valid {
			t.Fatalf("%+v valid=%v: %v", tc.s, tc.valid, err)
		}
	}
}
func TestRegisteredDefinitionDoesNotResolveDuringValidation(t *testing.T) {
	type Params struct {
		Label string `yaml:"label"`
	}
	calls := 0
	d := Define("third", "extension", configschema.Must[Params](), func(_ Params, _ Facts) (receivers.Plan, error) {
		calls++
		return receivers.Capture("third", "http://sink", "")
	}, nil)
	if err := d.validate(selection("third", "label: test")); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("schema registration/validation invoked runtime resolver")
	}
	if _, err := d.resolve(selection("third", ""), Facts{}); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
func TestExpectedFakeintakeNeverFallsBack(t *testing.T) {
	s := selection("fakeintake", "")
	for _, facts := range []Facts{{}, {FakeIntake: &outputs.FakeintakeOutput{URL: "http://127.0.0.1:8080"}}, {SeparateNetwork: true, FakeIntake: &outputs.FakeintakeOutput{AgentURL: "http://127.0.0.1"}}} {
		if _, err := Resolve(s, facts); err == nil {
			t.Fatal("missing/unsafe fixture must fail")
		}
	}
	p, err := Resolve(s, Facts{SeparateNetwork: true, FakeIntake: &outputs.FakeintakeOutput{AgentURL: "http://dev-fakeintake:80", QueryURL: "http://127.0.0.1:8080"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Endpoint != "http://dev-fakeintake:80" || p.QueryURL != "http://127.0.0.1:8080" {
		t.Fatal(p)
	}
}

// An empty url selects the environment-managed sink: resolution fails closed
// until the environment provides the fact, then routes exactly to it. A set
// url is the external escape hatch and ignores the managed fact.
func TestBlackholeManagedSelectionResolvesFromFacts(t *testing.T) {
	s := selection("blackhole", "")
	for _, facts := range []Facts{{}, {SeparateNetwork: true}, {Blackhole: &BlackholeOutput{}}, {FakeIntake: &outputs.FakeintakeOutput{AgentURL: "http://fixture:80"}}} {
		if _, err := Resolve(s, facts); err == nil {
			t.Fatalf("managed blackhole without a running sink fact must fail: %+v", facts)
		}
	}
	p, err := Resolve(s, Facts{SeparateNetwork: true, Blackhole: &BlackholeOutput{AgentURL: "http://dev-blackhole:8080"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Endpoint != "http://dev-blackhole:8080" || p.Type != "blackhole" {
		t.Fatal(p)
	}
	external, err := Resolve(selection("blackhole", "url: http://sink:8080"), Facts{Blackhole: &BlackholeOutput{AgentURL: "http://dev-blackhole:8080"}})
	if err != nil {
		t.Fatal(err)
	}
	if external.Endpoint != "http://sink:8080" {
		t.Fatalf("explicit url must win over the managed fact: %+v", external)
	}
	if _, err := Resolve(selection("blackhole", "url: http://127.0.0.1:8080"), Facts{SeparateNetwork: true}); err == nil {
		t.Fatal("producer-loopback sink URL must stay rejected")
	}
}

// Only a valid blackhole selection with an empty url asks the environment for
// a sink; every other shape stays the caller's own responsibility.
func TestManagedSinkRequiredPredicate(t *testing.T) {
	for _, tc := range []struct {
		s    *config.ReceiverSelection
		want bool
	}{
		{nil, false},
		{selection("fakeintake", ""), false},
		{selection("datadog", "site: datadoghq.eu\napi-key-ref: runner/api_key"), false},
		{selection("blackhole", "url: http://sink:8080"), false},
		{selection("blackhole", ""), true},
		{selection("blackhole", "{}"), true},
		// Invalid sections are reported by validation, never silently managed.
		{selection("blackhole", "url: not a url"), false},
		{selection("blackhole", "unknown: x"), false},
	} {
		if got := ManagedSinkRequired(tc.s); got != tc.want {
			t.Fatalf("%+v: managed=%v want %v", tc.s, got, tc.want)
		}
	}
}
