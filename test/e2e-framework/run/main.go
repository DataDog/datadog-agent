// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main is the entry point for the e2e-framework. It exposes the
// built-in scenarios from test/e2e-framework/registry.
//
// Runners that also need scenarios registered from another module (for example
// test/new-e2e/run, which blank-imports test/new-e2e/tests/*) have their own
// main package and call registry.Run() the same way.
package main

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/registry"
)

func main() {
	registry.Run()
}
