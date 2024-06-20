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
type logReceiver struct {
	lr integrations.Component
}

var logRcv logReceiver
var logReceiverMutex = sync.Mutex{}

// GetLogsReceiver returns a reference to the logs_receiver component for Python and Go checks to use.
func GetLogsReceiver() (integrations.Component, error) {
	logReceiverMutex.Lock()
	defer logReceiverMutex.Unlock()

	if logRcv.lr == nil {
		return nil, errors.New("logsReciever context was not set")
	}

	return logRcv.lr, nil
}

func LogsReceiverSendLog(log, logID string) {
	logReceiverMutex.Lock()
	logReceiverMutex.Unlock()

	logRcv.lr.SendLog(log, logID)
}

// InitializeLogsReceiver
func InitializeLogsReceiver(lr integrations.Component) {
	logReceiverMutex.Lock()
	defer logReceiverMutex.Unlock()

	if logRcv.lr == nil {
		logRcv.lr = lr
	}
}

// ReleaseLogReceiver resets to the log receiver to nil
func ReleaseLogReceiver() {
	logReceiverMutex.Lock()
	defer logReceiverMutex.Unlock()

	logRcv.lr = nil
}
