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

var lr integrations.Component
var logReceiverMutex = sync.Mutex{}

// GetLogsReceiver returns a reference to the logs_receiver component for Python and Go checks to use.
func GetLogsReceiver() (integrations.Component, error) {
	logReceiverMutex.Lock()
	defer logReceiverMutex.Unlock()

	if lr == nil {
		return nil, errors.New("logsReciever was not set")
	}

	return lr, nil
}

// LogsReceiverSendLog wraps the SendLog function on the integrations component.
func LogsReceiverSendLog(log, logID string) {
	logReceiverMutex.Lock()
	defer logReceiverMutex.Unlock()

	lr.SendLog(log, logID)
}

// InitializeLogsReceiver initializes the logreceiver component to be used later.
func InitializeLogsReceiver(logsReceiver integrations.Component) {
	logReceiverMutex.Lock()
	defer logReceiverMutex.Unlock()

	if lr == nil {
		lr = logsReceiver
	}
}

// ReleaseLogReceiver resets to the log receiver to nil
func ReleaseLogReceiver() {
	logReceiverMutex.Lock()
	defer logReceiverMutex.Unlock()

	lr = nil
}
