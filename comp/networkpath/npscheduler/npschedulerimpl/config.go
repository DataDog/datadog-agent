// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npschedulerimpl

// target can be an IP/CIDR
// in the future, we can possibly support more matching types, like hostname/protocol/etc
type target struct {
	IP string `json:"ip"` // IP or CIDR
}

// config is used to allow custom configure for precise targets (IP/CIDR/etc)
//
// In the future, we can possibly add more configuration options:
// - custom max TTL for specified targets
type pathtestConfig struct {
	Target  target `json:"target"`
	Enabled bool   `json:"enabled"` // default true
}
