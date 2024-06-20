// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"errors"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/integrations/def"
)

// checkContext holds a list of reference to different components used by Go and Python checks.
//
// This is a temporary solution until checks are components themselves and can request dependencies through FX.
//
// This also allows Go function exported to CPython to recover there reference to different components when coming out
// of C to Go. This way python checks can submit metadata to inventorychecks through the 'SetCheckMetadata' python
// method.
type logReceiverContext struct {
	lr integrations.Component
}

var logCtx logReceiverContext
var logReceiverContextMutex = sync.Mutex{}

// GetLogsReceiverContext returns a reference to the logs_receiver component for Python and Go checks to use.
func GetLogsReceiverContext() (integrations.Component, error) {
	checkContextMutex.Lock()
	defer checkContextMutex.Unlock()

	if logCtx.lr == nil {
		return nil, errors.New("logsReciever context was not set")
	}

	return logCtx.lr, nil
}

// InitializeInventoryChecksContext set the reference to inventorychecks in checkContext
func InitializeLogsReceiverContext(lr integrations.Component) {
	logReceiverContextMutex.Lock()
	defer logReceiverContextMutex.Unlock()

	if logCtx.lr == nil {
		logCtx.lr = lr
	}
}

// ReleaseContext reset to nil all the references hold by the current context
func ReleaseLogReceiverContext() {
	logReceiverContextMutex.Lock()
	defer logReceiverContextMutex.Unlock()

	logCtx.lr = nil
}
