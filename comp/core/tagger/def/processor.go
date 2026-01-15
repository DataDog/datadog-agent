// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagger provides the tagger interface for the Datadog Agent
package tagger

import "github.com/DataDog/datadog-agent/comp/core/tagger/types"

// Processor is an interface for replay specific tagging use-cases.
type Processor interface {
	ProcessTagInfo([]*types.TagInfo)
}
