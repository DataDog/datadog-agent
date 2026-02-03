// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

// Package probe holds probe related files
package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
)

// Opts defines some probe options
type Opts struct {
	// DontDiscardRuntime do not discard the runtime. Mostly used by functional tests
	DontDiscardRuntime bool
	// Tagger will override the default one. Mainly here for tests.
	Tagger tags.Tagger
}
