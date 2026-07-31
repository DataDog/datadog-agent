// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates the systemd units for the installer.
package main

import (
	"embed"
	"io/fs"
	"os"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
)

//go:embed gen
var genFS embed.FS

// TestGenerationIsUpToDate tests that the generated templates are up to date.
//
// You can update the templates by running `go generate` in the templates directory.
func TestGenerationIsUpToDate(t *testing.T) {
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("TestGenerationIsUpToDate is known to fail on the macOS Gitlab runners.")
	}

	generated := t.TempDir()
	err := generate(generated)
	assert.NoError(t, err)

	newGeneratedFS := os.DirFS(generated)
	currentGeneratedFS, err := fs.Sub(genFS, "gen")
	assert.NoError(t, err)

	fixtures.AssertEqualFS(t, currentGeneratedFS, newGeneratedFS)
}

// TestDDOTEnvVarsDefinedInProcmgrService tests that every environment variable
// referenced in datadog-agent-ddot.yaml.tmpl
// is defined in the datadog-agent-procmgr.service.tmpl context that starts the
// process manager responsible for running the ddot process.
func TestDDOTEnvVarsDefinedInProcmgrService(t *testing.T) {
	ddotYAML, err := os.ReadFile("datadog-agent-ddot.yaml.tmpl")
	assert.NoError(t, err)

	procmgrService, err := os.ReadFile("datadog-agent-procmgr.service.tmpl")
	assert.NoError(t, err)

	referencedVars := map[string]bool{}
	for _, match := range regexp.MustCompile(`\$\{(\w+)}`).FindAllStringSubmatch(string(ddotYAML), -1) {
		referencedVars[match[1]] = true
	}

	selfDefinedVars := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?m)^  (\w+):`).FindAllStringSubmatch(string(ddotYAML), -1) {
		selfDefinedVars[match[1]] = true
	}

	definedInService := map[string]bool{}
	for _, match := range regexp.MustCompile(`Environment="(\w+)=`).FindAllStringSubmatch(string(procmgrService), -1) {
		definedInService[match[1]] = true
	}

	for v := range referencedVars {
		if selfDefinedVars[v] {
			continue
		}
		assert.True(t, definedInService[v], "environment variable %q is used in datadog-agent-ddot.yaml.tmpl but not defined in datadog-agent-procmgr.service.tmpl", v)
	}
}

