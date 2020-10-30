// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	probe             *Probe
	DentryResolver    *DentryResolver
	MountResolver     *MountResolver
	ContainerResolver *ContainerResolver
	TimeResolver      *TimeResolver
	ProcessResolver   *ProcessResolver
}

// Start the resolvers
func (r *Resolvers) Start() error {
	if err := r.ProcessResolver.Start(); err != nil {
		return err
	}

	if err := r.MountResolver.Start(); err != nil {
		return err
	}

	return r.DentryResolver.Start()
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *Resolvers) Snapshot() error {
	return r.ProcessResolver.Snapshot(r.ContainerResolver, r.MountResolver)
}
