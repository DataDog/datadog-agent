// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package python

// DiscoveryService contains the service metadata sent to a Python integration's
// discovery bridge.
type DiscoveryService struct {
	ID    string          `json:"id"`
	Host  string          `json:"host"`
	Ports []DiscoveryPort `json:"ports"`
}

// DiscoveryPort contains a service port sent to a Python integration's discovery bridge.
type DiscoveryPort struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}
