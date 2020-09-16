// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"os"
	"syscall"

	"github.com/DataDog/gopsutil/process"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SnapshotTables - eBPF tables used by the kprobe used by the snapshot
var SnapshotTables = []string{
	"inode_numlower",
}

// SnapshotProbes lists of open's hooks
var SnapshotProbes = []*ebpf.KProbe{
	{
		Name:      "getattr",
		EntryFunc: "kprobe/vfs_getattr",
	},
}

// ProcCache this structure holds the container context that we keep in kernel for each process
type ProcCache struct {
	Inode    uint64
	Numlower uint32
	Padding  uint32
	ID       [utils.ContainerIDLen]byte
}

// Bytes returns the bytes representation of process cache entry
func (pc ProcCache) Bytes() []byte {
	b := make([]byte, 16+utils.ContainerIDLen)
	byteOrder.PutUint64(b[0:8], pc.Inode)
	byteOrder.PutUint32(b[8:12], pc.Numlower)
	copy(b[16:16+utils.ContainerIDLen], pc.ID[:])
	return b
}

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	probe            *Probe
	inodeNumlowerMap *ebpf.Table
	procCacheMap     *ebpf.Table
	pidCookieMap     *ebpf.Table

	DentryResolver    *DentryResolver
	MountResolver     *MountResolver
	ContainerResolver *ContainerResolver
	TimeResolver      *TimeResolver
}

// Start the resolvers
func (r *Resolvers) Start() error {
	// Select the in-kernel process cache that will be populated by the snapshot
	r.procCacheMap = r.probe.Table("proc_cache")
	if r.procCacheMap == nil {
		return errors.New("proc_cache BPF_HASH table doesn't exist")
	}

	// Select the in-kernel pid <-> cookie cache
	r.pidCookieMap = r.probe.Table("pid_cookie")
	if r.pidCookieMap == nil {
		return errors.New("pid_cookie BPF_HASH table doesn't exist")
	}

	if err := r.MountResolver.Start(); err != nil {
		return err
	}

	return r.DentryResolver.Start()
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *Resolvers) Snapshot(retry int) error {
	// Register snapshot tables
	for _, t := range SnapshotTables {
		err := r.probe.RegisterTable(t)
		if err != nil {
			return err
		}
	}

	// Select the inode numlower map to prepare for the snapshot
	r.inodeNumlowerMap = r.probe.Table("inode_numlower")
	if r.inodeNumlowerMap == nil {
		return errors.New("inode_numlower BPF_HASH table doesn't exist")
	}

	// Activate the probes required by the snapshot
	for _, kp := range SnapshotProbes {
		if err := r.probe.Module.RegisterKprobe(kp); err != nil {
			return errors.Wrapf(err, "couldn't register kprobe %s", kp.Name)
		}
	}

	err := r.snapshot(retry)

	// Deregister probes
	for _, kp := range SnapshotProbes {
		if err := r.probe.Module.UnregisterKprobe(kp); err != nil {
			log.Debugf("couldn't unregister kprobe %s: %v", kp.Name, err)
		}
	}

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
	byteOrder.PutUint32(pidb, pid)
	if _, err := r.pidCookieMap.Get(pidb); err == nil {
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
	byteOrder.PutUint64(inodeb, stat.Ino)
	numlowerb, err := r.inodeNumlowerMap.Get(inodeb)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't retrieve numlower value", pid))
		return false
	}
	entry.Numlower = byteOrder.Uint32(numlowerb)

	// Generate a new cookie for this pid
	byteOrder.PutUint32(cookieb, utils.NewCookie())

	// Insert the new cache entry and then the cookie
	if err := r.procCacheMap.SetP(cookieb, entry.Bytes()); err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't insert cache entry", pid))
		return false
	}
	if err := r.pidCookieMap.SetP(pidb, cookieb); err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't insert cookie", pid))
		return false
	}
	return true
}
