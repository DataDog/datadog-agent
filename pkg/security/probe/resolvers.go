// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"os"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/DataDog/gopsutil/process"
	"github.com/avast/retry-go"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	probe             *Probe
	DentryResolver    *DentryResolver
	MountResolver     *MountResolver
	ContainerResolver *ContainerResolver
	TimeResolver      *TimeResolver
	ProcessResolver   *ProcessResolver
	UserGroupResolver *UserGroupResolver
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(probe *Probe, client *statsd.Client) (*Resolvers, error) {
	dentryResolver, err := NewDentryResolver(probe)
	if err != nil {
		return nil, err
	}

	timeResolver, err := NewTimeResolver()
	if err != nil {
		return nil, err
	}

	userGroupResolver, err := NewUserGroupResolver()
	if err != nil {
		return nil, err
	}

	resolvers := &Resolvers{
		probe:             probe,
		DentryResolver:    dentryResolver,
		MountResolver:     NewMountResolver(probe),
		TimeResolver:      timeResolver,
		ContainerResolver: &ContainerResolver{},
		UserGroupResolver: userGroupResolver,
	}

	processResolver, err := NewProcessResolver(probe, resolvers, client)
	if err != nil {
		return nil, err
	}

	resolvers.ProcessResolver = processResolver

	return resolvers, nil
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
	// start the snapshot probes
	err := r.startSnapshotProbes()
	if err != nil {
		return err
	}

	// Deregister probes
	defer r.stopSnapshotProbes()

	if err := retry.Do(r.snapshot, retry.Delay(0), retry.Attempts(5)); err != nil {
		return errors.Wrap(err, "unable to snapshot processes")
	}

	return nil
}

// startSnapshotProbes starts the probes required for the snapshot to complete
func (r *Resolvers) startSnapshotProbes() error {
	// We only have process probes for now
	for _, sp := range r.ProcessResolver.GetProbes() {
		// enable and start the probe
		sp.Enabled = true
		if err := sp.Init(r.probe.manager); err != nil {
			return errors.Wrapf(err, "couldn't init probe %s", sp.GetIdentificationPair())
		}
		if err := sp.Attach(); err != nil {
			return errors.Wrapf(err, "couldn't start probe %s", sp.GetIdentificationPair())
		}
		log.Debugf("probe %s registered", sp.GetIdentificationPair())
	}
	return nil
}

// stopSnapshotProbes stops the snapshot probes
func (r *Resolvers) stopSnapshotProbes() {
	// We only have process probes for now
	for _, sp := range r.ProcessResolver.GetProbes() {
		// Stop snapshot probes
		if err := sp.Stop(); err != nil {
			log.Debugf("couldn't stop probe %s: %v", sp.GetIdentificationPair(), err)
		}
		// the probes selectors mechanism of the manager will re-enable this probe if needed
		sp.Enabled = false
		log.Debugf("probe %s stopped", sp.GetIdentificationPair())
	}
	return
}

// snapshot internal version of Snapshot. Calls the relevant resolvers to sync their caches.
func (r *Resolvers) snapshot() error {
	// List all processes, to trigger the process and mount snapshots
	processes, err := process.Pids()
	if err != nil {
		return err
	}

	cacheModified := false

	for _, pid := range processes {
		proc, err := process.NewProcess(pid)
		if err != nil {
			// the process does not exist anymore, continue
			continue
		}

		// Start with the mount resolver because the process resolver might need it to resolve paths
		if err := r.MountResolver.SyncCache(proc); err != nil {
			if !os.IsNotExist(err) {
				log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't sync mount points", proc.Pid))
			}
		}

		// Sync the process cache
		cacheModified = r.ProcessResolver.SyncCache(proc)
	}

	// There is a possible race condition when a process starts right after we called process.AllProcesses
	// and before we inserted the cache entry of its parent. Call Snapshot again until we do not modify the
	// process cache anymore
	if cacheModified {
		return errors.New("cache modified")
	}

	return nil
}
