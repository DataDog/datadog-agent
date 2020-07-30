// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	DentryResolver *DentryResolver
	MountResolver  *MountResolver
	TimeResolver   *TimeResolver
}

// Start the resolvers
func (r *Resolvers) Start() error {
	return r.DentryResolver.Start()
}
