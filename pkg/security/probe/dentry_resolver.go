// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

const (
	dentryPathKeyNotFound = "error: dentry path key not found"
)

// NewDentryResolver returns a new dentry resolver
func NewDentryResolver(probe *Probe) (*DentryResolver, error) {
	return &DentryResolver{
		probe: probe,
	}, nil
}
