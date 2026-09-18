// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import "testing"

func TestBuildSelectorOrderAndSectionOwnership(t *testing.T) {
	prefix := "schema: 1\nenvironment: {base: local}\nagent:\n  install: binary\n  build:\n"
	for _, build := range []string{"    provider: invoke-binary\n    invoke-binary: {repository: /source}\n", "    invoke-binary: {repository: /source}\n    provider: invoke-binary\n"} {
		cfg, errs := Parse([]byte(prefix + build))
		if len(errs) != 0 || cfg.Agent.Build.Provider != "invoke-binary" {
			t.Fatal(cfg, errs)
		}
	}
	for _, build := range []string{"    provider: 12\n", "    provider: existing-image\n    invoke-image: {}\n", "    reference: abc\n"} {
		if _, errs := Parse([]byte(prefix + build)); len(errs) == 0 {
			t.Fatal("accepted invalid selector", build)
		}
	}
}
