// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package replaytagger provides the replay tagger interface for the Datadog Agent
package replaytagger

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// team: container-platform

// Component is the component type wrapping around a regular tagger.Component
// with added replay-specific methods.
type Component interface {
	tagger.Component

	ProcessTagInfo([]*types.TagInfo)
}
