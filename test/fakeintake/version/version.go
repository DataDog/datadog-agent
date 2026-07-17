// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package version holds the pinned fakeintake image tag consumed by e2e tests.
//
// The tag is bumped in the same PR that changes fakeintake code (see
// test/fakeintake/AGENTS.md), so branches keep using a fixed, immutable image
// and only pick up a new fakeintake when they rebase onto a main that bumped
// the pin.
package version

import (
	"embed"
	"strings"
)

//go:embed VERSION
var versionFile embed.FS

// Tag is the pinned fakeintake image tag, read from the VERSION file.
var Tag = readTag()

func readTag() string {
	content, err := versionFile.ReadFile("VERSION")
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(content))
}
