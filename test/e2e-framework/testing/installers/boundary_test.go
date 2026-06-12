// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installers_test enforces the architectural boundary: installer
// packages must not DIRECTLY import Pulumi so they can be reused by the
// e2e-install CLI outside a Pulumi context.
//
// Note: transitive Pulumi dependencies (e.g. via agentparams legacy fields)
// are tolerated until Phase 4c removes them.
package installers_test

import (
	"go/build"
	"strings"
	"testing"
)

// forbiddenDirectImports are prefixes that must not appear in the DIRECT
// imports of installer packages (not their transitive deps).
var forbiddenDirectImports = []string{
	"github.com/pulumi/",
}

func TestInstallersNoPulumiDirectImport(t *testing.T) {
	installerPkgs := []string{
		"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent",
		"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent",
		"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/workloads",
	}

	for _, pkgPath := range installerPkgs {
		t.Run(pkgPath, func(t *testing.T) {
			pkg, err := build.Default.Import(pkgPath, ".", 0)
			if err != nil {
				t.Skipf("cannot resolve package %s: %v", pkgPath, err)
			}
			for _, imp := range pkg.Imports {
				for _, forbidden := range forbiddenDirectImports {
					if strings.HasPrefix(imp, forbidden) {
						t.Errorf("installer package %s directly imports forbidden package %s; "+
							"installer packages must not directly depend on Pulumi so they can run outside a Pulumi context",
							pkgPath, imp)
					}
				}
			}
		})
	}
}
