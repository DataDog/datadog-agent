// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package id contains the ID for a check
package id

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// ID is the representation of the unique ID of a Check instance
type ID string

// checkNameInterner deduplicates check name strings so that all instances of
// the same check type share a single backing string allocation.  The set of
// distinct check names on any given host is tiny (O(tens)), so the map never
// grows large enough to need eviction.
var (
	checkNameInternerMu sync.Mutex
	checkNameInternerM  = make(map[string]string)
)

// InternCheckName returns a deduplicated copy of s, allocating a new entry only
// on the first call for each distinct value.  The set of check names on any
// given host is tiny (O(tens)), so the map never grows large enough to need
// eviction.  Callers that store a check name for the lifetime of a check
// instance (e.g. CheckBase.checkName) should use this to avoid holding
// redundant string allocations when many instances of the same check run.
func InternCheckName(s string) string {
	checkNameInternerMu.Lock()
	defer checkNameInternerMu.Unlock()
	if v, ok := checkNameInternerM[s]; ok {
		return v
	}
	checkNameInternerM[s] = s
	return s
}

// BuildID returns an unique ID for a check name and its configuration
func BuildID(checkName string, integrationConfigDigest uint64, instance, initConfig integration.Data) ID {
	checkName = InternCheckName(checkName)
	// Hash is returned in BigEndian
	digestBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(digestBytes, integrationConfigDigest)

	h := fnv.New64()
	_, _ = h.Write(digestBytes)
	_, _ = h.Write([]byte(instance))
	_, _ = h.Write([]byte(initConfig))
	name := instance.GetNameForInstance()

	if name != "" {
		return ID(fmt.Sprintf("%s:%s:%x", checkName, name, h.Sum64()))
	}

	return ID(fmt.Sprintf("%s:%x", checkName, h.Sum64()))
}

// IDToCheckName returns the check name from a check ID
//
//nolint:revive
func IDToCheckName(id ID) string {
	return strings.SplitN(string(id), ":", 2)[0]
}
