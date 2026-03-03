// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	collectoraggregator "github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

static inline void* call_malloc(size_t sz) {
    return _malloc(sz);
}
*/
import "C"

// Tags bridges towards tagger.Tag to retrieve container tags
//
//export Tags
func Tags(id *C.char, cardinality C.int) **C.char {
	checkContext, err := collectoraggregator.GetCheckContext()
	if err != nil {
		log.Errorf("Python check context: %v", err)
		return nil
	}

	goID := C.GoString(id)
	var tags []string

	// Generate EntityID from the entity ID string.
	// This is done for backward compatibility with the Python checks as the Tags() signature cannot be changed.
	prefix, eid, err := types.ExtractPrefixAndID(goID)
	if err != nil {
		log.Errorf("could not extract prefix and ID from id string: %v. Using LegacyTag.", err)
		return nil
	}
	entityID := types.NewEntityID(prefix, eid)

	tags, _ = checkContext.Tag(entityID, types.TagCardinality(cardinality))

	length := len(tags)
	if length == 0 {
		return nil
	}

	cTags := C.call_malloc(C.size_t(uintptr(length+1) * unsafe.Sizeof(uintptr(0))))
	if cTags == nil {
		log.Errorf("could not allocate memory for tags")
		return nil
	}

	// convert the C array to a Go Array so we can index it
	indexTag := (*[1<<29 - 1]*C.char)(cTags)[: length+1 : length+1]
	indexTag[length] = nil
	for idx, tag := range tags {
		indexTag[idx] = TrackedCString(tag)
	}

	return (**C.char)(cTags)
}
