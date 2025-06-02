// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package netlink

import "github.com/vishvananda/netns"

// NewNoOpConntrack returns a new no-op conntrack implementation.
func NewNoOpConntrack(_ netns.NsHandle) (Conntrack, error) {
	return &noOpConntrack{}, nil
}

// IsNoOpConntrack checks if a Conntrack instance is a no-op.
// This is used to verify behavior during tests
func IsNoOpConntrack(ct Conntrack) bool {
	_, ok := ct.(*noOpConntrack)
	return ok
}

type noOpConntrack struct{}

var _ Conntrack = &noOpConntrack{}

func (c *noOpConntrack) Close() error {
	return nil
}

func (c *noOpConntrack) Exists(_ *Con) (bool, error) {
	return false, nil
}
