// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package health

import (
	"errors"
	"time"
)

var globalCatalog = newCatalog()

// Register a component with the default 30 seconds timeout, returns a token
func Register(name string) *Handle {
	return globalCatalog.register(name)
}

// Deregister a component from the healthcheck
func Deregister(handle *Handle) error {
	return globalCatalog.deregister(handle)
}

// GetStatus allows to query the health status of the agent
func GetStatus() Status {
	return globalCatalog.getStatus()
}

// GetStatusNonBlocking allows to query the health status of the agent
// and is guaranteed to return under 500ms.
func GetStatusNonBlocking() (Status, error) {
	// Run the health status in a goroutine
	ch := make(chan Status, 1)
	go func() {
		ch <- GetStatus()
	}()

	// Only wait 500ms before returning
	select {
	case status := <-ch:
		return status, nil
	case <-time.After(500 * time.Millisecond):
		return Status{}, errors.New("timeout when getting health status")
	}
}
