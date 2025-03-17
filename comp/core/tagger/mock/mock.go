// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock contains the interface of the mock for the tagger component.
package mock

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// Mock implements mock-specific methods for the tagger component.
type Mock interface {
	tagger.Component

	// GetTagStore returns the tag store
	GetTagStore() *tagstore.TagStore

	// SetTags allows to set tags in the mock fake tagger
	SetTags(entityID types.EntityID, source string, low, orch, high, std []string)

	// SetGlobalTags allows to set tags in store for the global entity
	SetGlobalTags(low, orch, high, std []string)

	// LoadState loads the state for the tagger from the supplied map.
	LoadState(state []types.Entity)
}
