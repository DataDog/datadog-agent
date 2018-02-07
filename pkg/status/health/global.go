// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package health

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
