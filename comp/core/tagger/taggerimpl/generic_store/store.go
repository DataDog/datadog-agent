// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package genericstore

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// NewObjectStore constructs and returns a an ObjectStore
func NewObjectStore[T any](cfg config.Component) types.ObjectStore[T] {
	// TODO: use composite object store always or use component framework for config component
	if cfg.GetBool("tagger.tagstore_use_composite_entity_id") {
		return newCompositeObjectStore[T]()
	}
	return newDefaultObjectStore[T]()
}
