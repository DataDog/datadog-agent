// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/DataDog/gopsutil/process"
	"github.com/pkg/errors"
	"os"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcCache this structure holds the container context that we keep in kernel for each process
type ProcCache struct {
	Inode    uint64
	Numlower uint32
	Padding  uint32
	ID       [utils.ContainerIDLen]byte
}

// Bytes returns the bytes representation of process cache entry
func (pc ProcCache) MarshalBinary() []byte {
	b := make([]byte, 16+utils.ContainerIDLen)
	ebpf.ByteOrder.PutUint64(b[0:8], pc.Inode)
	ebpf.ByteOrder.PutUint32(b[8:12], pc.Numlower)
	copy(b[16:16+utils.ContainerIDLen], pc.ID[:])
	return b
}

// snapshotProbeIDs holds the list of probes that are required for the snapshot
var snapshotProbeIDs = []manager.ProbeIdentificationPair{
	{
		UID:     probes.SecurityAgentUID,
		Section: "kprobe/security_inode_getattr",
	},
}

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	probe            *Probe
	snapshotProbes   []*manager.Probe
	inodeNumlowerMap *lib.Map
	procCacheMap     *lib.Map
	pidCookieMap     *lib.Map

	DentryResolver    *DentryResolver
	MountResolver     *MountResolver
	ContainerResolver *ContainerResolver
	TimeResolver      *TimeResolver
}

// Init the resolvers
func (r *Resolvers) Init() error {
	// initializes the list of snapshot probes
	for _, id := range snapshotProbeIDs {
		p, ok := r.probe.manager.GetProbe(id)
		if !ok {
			return errors.Errorf("couldn't find probe %s", id)
		}
		r.snapshotProbes = append(r.snapshotProbes, p)
	}

	// select maps
	r.inodeNumlowerMap = r.probe.Map("inode_numlower")
	if r.inodeNumlowerMap == nil {
		return errors.New("map inode_numlower not found")
	}
	r.procCacheMap = r.probe.Map("proc_cache")
	if r.procCacheMap == nil {
		return errors.New("map proc_cache not found")
	}
	r.pidCookieMap = r.probe.Map("pid_cookie")
	if r.pidCookieMap == nil {
		return errors.New("map pid_cookie not found")
	}

	if err := r.MountResolver.Start(); err != nil {
		return err
	}

	return r.DentryResolver.Start()
}

// startSnapshotProbes starts the probes required for the snapshot to complete
func (r *Resolvers) startSnapshotProbes() error {
	for _, p := range r.snapshotProbes {
		// enable and start the probe
		p.Enabled = true
		if err := p.Init(r.probe.manager); err != nil {
			return errors.Wrapf(err, "couldn't init probe %s", p.GetIdentificationPair())
		}
		if err := p.Attach(); err != nil {
			return errors.Wrapf(err, "couldn't start probe %s", p.GetIdentificationPair())
		}
		log.Debugf("probe %s registered", p.GetIdentificationPair())
	}
	return nil
}

// stopSnapshotProbes stops the snapshot probes
func (r *Resolvers) stopSnapshotProbes() {
	for _, p := range r.snapshotProbes {
		// Stop snapshot probes
		if err := p.Stop(); err != nil {
			log.Debugf("couldn't stop probe %s: %v", p.GetIdentificationPair(), err)
		}
		// the probes selectors mechanism of the manager will re-enable this probe if needed
		p.Enabled = false
		log.Debugf("probe %s stopped", p.GetIdentificationPair())
	}
	return
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *Resolvers) Snapshot(retry int) error {
	// start the snapshot probes
	if err := r.startSnapshotProbes(); err != nil {
		return err
	}

	// Select the inode numlower map to prepare for the snapshot
	r.inodeNumlowerMap = r.probe.Map("inode_numlower")
	if r.inodeNumlowerMap == nil {
		return errors.New("inode_numlower BPF_HASH table doesn't exist")
	}

	err := r.snapshot(retry)

	// stop and cleanup the snapshot probes
	r.stopSnapshotProbes()

	return err
}

func (r *Resolvers) snapshot(retry int) error {
	if retry <= 0 {
		return nil
	}

	// List all processes
	processes, err := process.AllProcesses()
	if err != nil {
		return err
	}

	cacheModified := false

	for _, p := range processes {
		// If Exe is not set, the process is a short lived process and its /proc entry has already expired, move on.
		if len(p.Exe) == 0 {
			continue
		}

		// Notify that we modified the cache.
		if r.snapshotProcess(uint32(p.Pid)) {
			cacheModified = true
		}
	}

	// There is a possible race condition where a process could have started right after we did the call to
	// process.AllProcesses and before we inserted the cache entry of its parent. Call Snapshot again until we
	// do not modify the process cache anymore
	if cacheModified {
		retry--
		return r.snapshot(retry)
	}
	return nil
}

// snapshotProcess snapshots /proc for the provided pid. This method returns true if it updated the kernel process cache.
func (r *Resolvers) snapshotProcess(pid uint32) bool {
	entry := ProcCache{}
	pidb := make([]byte, 4)
	cookieb := make([]byte, 4)
	inodeb := make([]byte, 8)

	// Check if there already is an entry in the pid <-> cookie cache
	ebpf.ByteOrder.PutUint32(pidb, pid)
	if value, _ := r.pidCookieMap.LookupBytes(pidb); len(value) > 0 {
		// If there is a cookie, there is an entry in cache, we don't need to do anything else
		return false
	}

	// Populate the mount point cache for the process
	if err := r.MountResolver.SyncCache(pid); err != nil {
		if !os.IsNotExist(err) {
			log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't sync mount points", pid))
			return false
		}
	}

	// Retrieve the container ID of the process
	containerID, err := r.ContainerResolver.GetContainerID(pid)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't parse container ID", pid))
		return false
	}
	entry.ID = containerID.Bytes()

	// Get the inode of the process binary
	fi, err := os.Stat(utils.ProcExePath(pid))
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't stat binary", pid))
		return false
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't stat binary", pid))
		return false
	}
	entry.Inode = stat.Ino

	// Fetch the numlower value of the inode
	ebpf.ByteOrder.PutUint64(inodeb, stat.Ino)
	numlowerb, err := r.inodeNumlowerMap.LookupBytes(inodeb)
	if err != nil || len(numlowerb) == 0 {
		if err == nil {
			err = errors.Errorf("inode %d for binary %s not found in map", stat.Ino, fi.Name())
		}
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't retrieve numlower value of inode %d", pid, stat.Ino))
		return false
	}

	// Generate a new cookie for this pid
	ebpf.ByteOrder.PutUint32(cookieb, utils.NewCookie())

	// Insert the new cache entry and then the cookie
	if err := r.procCacheMap.Put(cookieb, entry); err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't insert cache entry", pid))
		return false
	}
	if err := r.pidCookieMap.Put(pidb, cookieb); err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't insert cookie", pid))
		return false
	}
	return true
}
