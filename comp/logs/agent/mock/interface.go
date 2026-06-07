// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package mock

import (
	agent "github.com/DataDog/datadog-agent/comp/logs/agent/def"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Mock implements mock-specific methods.
type Mock interface {
	agent.Component

	SetSources(sources *sources.LogSources)
}
