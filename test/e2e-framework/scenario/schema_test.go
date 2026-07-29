// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"strings"
	"testing"
)

type compChild struct {
	Flavor string `scenario:"name=agent-flavor,enum=datadog-agent|datadog-fips-agent"`
}

type schemaSample struct {
	OS       string    `scenario:"name=os,default=ubuntu-22.04,help=Operating system,enum=ubuntu-22.04|debian-12"`
	Replicas int       `scenario:"name=replicas,default=1"`
	Verbose  bool      `scenario:"name=verbose"`
	Required string    `scenario:"name=token,required"`
	Child    compChild // embedded component, recurse
	Hidden   []string  `scenario:"-"`            // escape hatch, skipped
	Untagged string                              // no tag, skipped
}

// dupParent has two fields that resolve to the same flag name "dup-flag".
type dupChild1 struct {
	A string `scenario:"name=dup-flag"`
}
type dupChild2 struct {
	B string `scenario:"name=dup-flag"`
}
type dupParent struct {
	dupChild1
	dupChild2
}

func TestBuildSchemaRejectsDuplicateNames(t *testing.T) {
	_, err := BuildSchema(&dupParent{})
	if err == nil {
		t.Fatal("expected error for duplicate flag name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate scenario flag name") {
		t.Errorf("error message unexpected: %v", err)
	}
}

func TestBuildSchema(t *testing.T) {
	s, err := BuildSchema(&schemaSample{})
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}
	byName := map[string]Field{}
	for _, f := range s.Fields {
		byName[f.Name] = f
	}
	if len(byName) != 5 {
		t.Fatalf("want 5 fields, got %d (%v)", len(byName), s.Fields)
	}
	os := byName["os"]
	if os.Kind != KindString || os.Default != "ubuntu-22.04" || os.Help != "Operating system" {
		t.Errorf("os field wrong: %+v", os)
	}
	if len(os.Enum) != 2 || os.Enum[0] != "ubuntu-22.04" {
		t.Errorf("os enum wrong: %v", os.Enum)
	}
	if byName["replicas"].Kind != KindInt {
		t.Errorf("replicas kind wrong: %v", byName["replicas"].Kind)
	}
	if byName["verbose"].Kind != KindBool {
		t.Errorf("verbose kind wrong: %v", byName["verbose"].Kind)
	}
	if !byName["token"].Required {
		t.Errorf("token should be required")
	}
	if _, ok := byName["agent-flavor"]; !ok {
		t.Errorf("nested component field agent-flavor missing")
	}
	if _, ok := byName["Hidden"]; ok {
		t.Errorf("escape-hatch field must be skipped")
	}
}
