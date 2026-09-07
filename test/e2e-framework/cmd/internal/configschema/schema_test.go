// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configschema

import (
	"fmt"
	"strings"
	"testing"
)

type sampleConfig struct {
	OS      string `yaml:"os" config:"required" enum:"linux,windows" example:"linux" description:"Target OS."`
	Arch    string `yaml:"arch" enum:"amd64,arm64" default:"amd64"`
	Enabled bool   `yaml:"enabled,omitempty" default:"true"`
	Nodes   int    `yaml:"nodes,omitempty" default:"2" minimum:"0" maximum:"5"`
	AMI     string `yaml:"ami,omitempty" pattern:"^ami-[a-z0-9]+$"`
}

type customDecoder string

func (d *customDecoder) UnmarshalText(_ []byte) error {
	*d = "silently-replaced"
	return nil
}

func TestCustomDecodersCannotBypassValidation(t *testing.T) {
	if _, err := Compile[struct {
		Mode customDecoder `yaml:"mode"`
	}](); err == nil {
		t.Fatal("custom codecs need explicit schema support, not hidden post-validation decoding")
	}
}

func TestResolvedPayloadRequiresExplicitDefaults(t *testing.T) {
	type parent struct {
		Ranges map[string]rangeConfig `yaml:"ranges"`
	}
	s := Must[parent]()
	if _, err := s.DecodeResolved([]byte("ranges: {first: {}}"), "wire"); err == nil {
		t.Fatal("nested defaults must already be materialized")
	}
	p, raw, err := s.Decode([]byte("ranges: {first: {low: 0}}"), "user")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.DecodeResolved(raw, "wire")
	if err != nil || got.Ranges["first"] != p.Ranges["first"] {
		t.Fatalf("resolved roundtrip failed: %+v %v", got, err)
	}
}

func TestDefaultsPresenceAndWireRoundtrip(t *testing.T) {
	s := Must[sampleConfig]()
	value, raw, err := s.Decode([]byte("os: linux\nenabled: false\nnodes: 0\n"), "environment")
	if err != nil {
		t.Fatal(err)
	}
	if value.Arch != "amd64" || value.Enabled || value.Nodes != 0 {
		t.Fatalf("wrong defaults: %+v", value)
	}
	if !strings.Contains(string(raw), "enabled: false") || !strings.Contains(string(raw), "nodes: 0") {
		t.Fatalf("wire payload lost explicit zero/false despite omitempty: %s", raw)
	}
	receiver, _, err := s.Decode(raw, "executor")
	if err != nil || receiver != value {
		t.Fatalf("process boundaries disagree: %+v %v", receiver, err)
	}
	omitted, _, err := s.Decode([]byte("os: linux\n"), "environment")
	if err != nil {
		t.Fatal(err)
	}
	if !omitted.Enabled || omitted.Nodes != 2 {
		t.Fatalf("defaults were not applied: %+v", omitted)
	}
	if _, _, err := s.Decode(nil, "environment"); err == nil || !strings.Contains(err.Error(), "os") {
		t.Fatalf("example must not act as runtime default: %v", err)
	}
}

func TestValidationAndLocations(t *testing.T) {
	s := Must[sampleConfig]()
	for name, raw := range map[string]string{
		"unknown": "os: linux\nnope: true", "duplicate": "os: linux\nos: windows",
		"extra doc": "os: linux\n---\nos: windows", "enum": "os: macos", "empty": "os: ''",
		"null": "os: null", "type": "os: linux\nnodes: '0'", "min": "os: linux\nnodes: -1",
		"max": "os: linux\nnodes: 6", "pattern": "os: linux\nami: invalid",
		"alias": "os: &os linux\narch: *os", "map keys": "os: linux\n42: x",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := s.Decode([]byte(raw), "config.yaml: environment"); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	_, _, err := s.Decode([]byte("os: linux\n\nnodes: -1\narch: invalid\n"), "config.yaml: environment")
	if err == nil {
		t.Fatal("expected independent field errors")
	}
	for _, want := range []string{"environment.nodes", "line 3", "environment.arch", "line 4"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in diagnostic: %v", want, err)
		}
	}
}

func TestRequiredFalseAndZero(t *testing.T) {
	type params struct {
		Flag  bool `yaml:"flag" config:"required"`
		Count int  `yaml:"count" config:"required"`
	}
	if _, _, err := Must[params]().Decode([]byte("flag: false\ncount: 0"), "input"); err != nil {
		t.Fatal(err)
	}
}

type rangeConfig struct {
	Low  int `yaml:"low" default:"1" minimum:"0"`
	High int `yaml:"high" default:"5" minimum:"0"`
}

type rangeValidator struct {
	calls int
	seen  rangeConfig
}

func (v *rangeValidator) Validate(p rangeConfig) error {
	v.calls++
	v.seen = p
	if p.Low > p.High {
		return fmt.Errorf("low must not exceed high")
	}
	return nil
}

func TestOptionalValidateAfterAutomaticValidation(t *testing.T) {
	v := &rangeValidator{}
	s := Must[rangeConfig](v)
	p, raw, err := s.Decode(nil, "input")
	if err != nil || v.calls != 1 || v.seen.Low != 1 || v.seen.High != 5 {
		t.Fatalf("hook did not see defaults: %+v %v", v, err)
	}
	if _, _, err := s.Decode([]byte("low: -1"), "input"); err == nil || v.calls != 1 {
		t.Fatal("hook must not run on invalid automatic input")
	}
	if _, _, err := s.Decode([]byte("low: 6\nhigh: 5"), "input"); err == nil || !strings.Contains(err.Error(), "low must not exceed high") {
		t.Fatalf("semantic rule did not run: %v", err)
	}
	// The executor can use the same rule implementation with the same config type.
	receiverRule := &rangeValidator{}
	got, _, err := Must[rangeConfig](receiverRule).Decode(raw, "executor")
	if err != nil || got != p || receiverRule.calls != 1 {
		t.Fatalf("shared boundary validation failed: %v", err)
	}
	if _, err := s.Example(nil); err != nil {
		t.Fatal(err)
	}
}

func TestNestedCollectionsAndPointers(t *testing.T) {
	type nested struct {
		Range  rangeConfig       `yaml:"range" config:"required"`
		Tags   map[string]string `yaml:"tags,omitempty"`
		Values []int             `yaml:"values,omitempty"`
		Name   *string           `yaml:"name,omitempty"`
	}
	p, _, err := Must[nested]().Decode([]byte("range: {}\ntags: {team: agent}\nvalues: [0, 2]\nname: hello\n"), "input")
	if err != nil {
		t.Fatal(err)
	}
	if p.Range.Low != 1 || p.Name == nil || *p.Name != "hello" || len(p.Values) != 2 {
		t.Fatalf("wrong nested decode: %+v", p)
	}
	if _, _, err := Must[nested]().Decode([]byte("range: {}\ntags: {team: 42}"), "input"); err == nil {
		t.Fatal("map value type was not validated")
	}
}

func TestExampleIsAnnotatedValidatedAndIndependent(t *testing.T) {
	s := Must[sampleConfig]()
	n, err := s.Example(nil)
	if err != nil {
		t.Fatal(err)
	}
	text, err := Encode(n)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "Target OS.") || !strings.Contains(string(text), "Example value") || !strings.Contains(string(text), "Default value") {
		t.Fatalf("missing generated comments: %s", text)
	}
	if _, _, err := s.Decode(text, "example"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Example(map[string]any{"os": "invalid"}); err == nil {
		t.Fatal("invalid override accepted")
	}
	if _, err := s.Example(map[string]any{"unknown": "x"}); err == nil {
		t.Fatal("unknown example field accepted")
	}
	n.Content[1].Value = "mutated"
	if _, err := s.Example(nil); err != nil {
		t.Fatalf("example mutation corrupted compiled metadata: %v", err)
	}
}

func TestSchemaErrors(t *testing.T) {
	tests := []struct {
		name    string
		compile func() error
	}{
		{"default enum", func() error {
			_, err := Compile[struct {
				Arch string `yaml:"arch" enum:"amd64,arm64" default:"wrong"`
			}]()
			return err
		}},
		{"example bound", func() error {
			_, err := Compile[struct {
				N int `yaml:"n" minimum:"0" example:"-1"`
			}]()
			return err
		}},
		{"invalid pattern", func() error {
			_, err := Compile[struct {
				S string `yaml:"s" pattern:"["`
			}]()
			return err
		}},
		{"unknown flag", func() error {
			_, err := Compile[struct {
				S string `yaml:"s" config:"requird"`
			}]()
			return err
		}},
		{"unsupported type", func() error {
			_, err := Compile[struct {
				F func() `yaml:"f"`
			}]()
			return err
		}},
		{"duplicate names", func() error {
			_, err := Compile[struct {
				A string `yaml:"x"`
				B string `yaml:"x"`
			}]()
			return err
		}},
		{"secret example", func() error {
			_, err := Compile[struct {
				S string `yaml:"secret" config:"secret" example:"nope"`
			}]()
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.compile() == nil {
				t.Fatal("invalid schema was accepted")
			}
		})
	}
}

type secretChild struct {
	Token string `yaml:"token,omitempty" config:"secret"`
	Name  string `yaml:"name,omitempty"`
}

func TestNestedSecretsCannotBecomeDefaultsOrExamples(t *testing.T) {
	cases := []struct {
		name    string
		compile func() error
	}{
		{"struct example", func() error {
			_, err := Compile[struct {
				Child secretChild `yaml:"child" example:"{token: do-not-emit}"`
			}]()
			return err
		}},
		{"struct default", func() error {
			_, err := Compile[struct {
				Child secretChild `yaml:"child" default:"{token: do-not-emit}"`
			}]()
			return err
		}},
		{"map example", func() error {
			_, err := Compile[struct {
				Children map[string]secretChild `yaml:"children" example:"{first: {token: do-not-emit}}"`
			}]()
			return err
		}},
		{"slice example", func() error {
			_, err := Compile[struct {
				Children []secretChild `yaml:"children" example:"[{token: do-not-emit}]"`
			}]()
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.compile()
			if err == nil {
				t.Fatal("nested secret was accepted as generated metadata")
			}
			if strings.Contains(err.Error(), "do-not-emit") {
				t.Fatal("diagnostic leaked the secret")
			}
		})
	}
}

func TestNestedSecretExampleOverridesAreRejected(t *testing.T) {
	type parent struct {
		Child    secretChild            `yaml:"child,omitempty"`
		Children map[string]secretChild `yaml:"children,omitempty"`
		List     []secretChild          `yaml:"list,omitempty"`
	}
	s := Must[parent]()
	for _, override := range []map[string]any{
		{"child": map[string]string{"token": "do-not-emit"}},
		{"children": map[string]any{"first": map[string]string{"token": "do-not-emit"}}},
		{"list": []map[string]string{{"token": "do-not-emit"}}},
	} {
		if _, err := s.Example(override); err == nil {
			t.Fatal("nested override exposed a secret")
		}
	}
	if _, err := s.Example(map[string]any{"child": map[string]string{"name": "safe"}}); err != nil {
		t.Fatal(err)
	}
}

func TestNestedExamplesRenderAsBlocks(t *testing.T) {
	type parent struct {
		Range rangeConfig `yaml:"range" example:"{low: 1, high: 5}"`
	}
	n, err := Must[parent]().Example(nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Encode(n)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "range:\n  low: 1\n  high: 5") {
		t.Fatalf("expected editable multi-line YAML: %s", data)
	}
}

func TestMissingRequiredExample(t *testing.T) {
	type cfg struct {
		Name string `yaml:"name" config:"required"`
	}
	s := Must[cfg]()
	if _, err := s.Example(nil); err == nil {
		t.Fatal("must not invent a required value")
	}
	if _, err := s.Example(map[string]any{"name": "example"}); err != nil {
		t.Fatal(err)
	}
}
