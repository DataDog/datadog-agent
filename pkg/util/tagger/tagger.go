// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagger provides function to check if the tagger should use composite entity id and object store
package tagger

import (
	"sync"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

var useCompositeStore bool
var doOnce sync.Once

// ShouldUseCompositeStore indicates whether the tagger should use the default or composite implementation
// of entity ID and object store.
// TODO: remove this when we switch over fully to the composite implementation
func ShouldUseCompositeStore() bool {
	doOnce.Do(func() {
		useCompositeStore = pkgconfigsetup.Datadog().GetBool("tagger.tagstore_use_composite_entity_id")
	})
	return useCompositeStore
}
