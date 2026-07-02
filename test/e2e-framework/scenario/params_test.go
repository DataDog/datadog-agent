// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import "testing"

type defaultsSample struct {
	OS      string `scenario:"name=os,default=ubuntu-22.04"`
	Count   int    `scenario:"name=count,default=3"`
	Enabled bool   `scenario:"name=enabled,default=true"`
	NoDef   string `scenario:"name=nodef"`
}

func TestApplyDefaults(t *testing.T) {
	var d defaultsSample
	s, _ := BuildSchema(&d)
	if err := ApplyDefaults(s, &d); err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}
	if d.OS != "ubuntu-22.04" || d.Count != 3 || !d.Enabled || d.NoDef != "" {
		t.Fatalf("defaults wrong: %+v", d)
	}
}

func TestNewParamsIsDefaulted(t *testing.T) {
	p := NewParams[defaultsSample]()
	if p.OS != "ubuntu-22.04" || p.Count != 3 || !p.Enabled {
		t.Fatalf("NewParams not defaulted: %+v", p)
	}
}
