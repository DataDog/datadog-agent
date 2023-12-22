// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"testing"
)

// Context defines an interface that allows to get information about current test context
type Context interface {
	T() *testing.T
}

// Initializable defines the interface for an object that needs to be initialized
type Initializable interface {
	Init(Context) error
}

// RawResources is the common types returned by provisioners
type RawResources map[string][]byte

// Merge merges two RawResources maps
func (rr RawResources) Merge(in RawResources) {
	for k, v := range in {
		rr[k] = v
	}
}
