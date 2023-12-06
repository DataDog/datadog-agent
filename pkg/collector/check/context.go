// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"errors"
	"sync"

	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
)

// checkContext holds a list of reference to different components used by Go and Python checks.
//
// This is a temporary solution until checks are components themselves and can request dependencies through FX.
//
// This also allows Go function exported to CPython to recover there reference to different components when coming out
// of C to Go. This way python checks can submit metadata to inventorychecks through the 'SetCheckMetadata' python
// method.
type checkContext struct {
	ic inventorychecks.Component
}

var ctx checkContext
var checkContextMutex = sync.Mutex{}

// GetInventoryChecksContext returns a reference to the inventorychecks component for Python and Go checks to use.
func GetInventoryChecksContext() (inventorychecks.Component, error) {
	checkContextMutex.Lock()
	defer checkContextMutex.Unlock()

	if ctx.ic == nil {
		return nil, errors.New("inventorychecks context was not set")
	}
	return ctx.ic, nil
}

// InitializeInventoryChecksContext set the reference to inventorychecks in checkContext
func InitializeInventoryChecksContext(ic inventorychecks.Component) {
	checkContextMutex.Lock()
	defer checkContextMutex.Unlock()

	if ctx.ic == nil {
		ctx.ic = ic
	}
}

// ReleaseContext reset to nil all the references hold by the current context
func ReleaseContext() {
	checkContextMutex.Lock()
	defer checkContextMutex.Unlock()

	ctx.ic = nil
}
