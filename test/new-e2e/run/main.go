// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main is the Pulumi entry point for demo lab scenarios.
//
// The scenario resolution and dispatch logic lives in registry.Run, shared with
// test/e2e-framework/run. This runner exists separately only because it must
// blank-import scenarios from this module: test/new-e2e depends on
// test/e2e-framework, so the framework's own runner cannot import back into
// test/new-e2e/tests/*.
//
// It therefore exposes all scenarios registered via registry.RegisterScenario
// (called from init() in test/new-e2e/tests/*/scenario.go files), as well as the
// built-in scenarios from test/e2e-framework/registry.
//
// Regenerate scenarios_import_gen.go after adding a new scenario.go:
//
//go:generate go run ../../../tools/generate-scenario-imports/main.go
package main

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/registry"
)

func main() {
	registry.Run()
}
