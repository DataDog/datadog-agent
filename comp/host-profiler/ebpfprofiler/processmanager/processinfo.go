// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// # Missing
//
// - Add periodic mapping synchronization for tracked PIDs.
// - Handle reported PIDs at finer per-PID granularity.

package processmanager

// This file is the only place that should access pidToProcessInfo.
// The map is used to synchronize state between eBPF maps and process
// manager. The access needs to stay here so the interaction between
// these two components can be audited to be consistent.

// The public functions in this file are restricted to be used from the
// HA/tracer and tools/coredump modules only.

import (
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"sort"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/internal/log"
	"golang.org/x/sys/unix"

	"go.opentelemetry.io/ebpf-profiler/host"
	"go.opentelemetry.io/ebpf-profiler/interpreter"
	"go.opentelemetry.io/ebpf-profiler/libc"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"
	"go.opentelemetry.io/ebpf-profiler/lpm"
	"go.opentelemetry.io/ebpf-profiler/process"
	"go.opentelemetry.io/ebpf-profiler/reporter"
	"go.opentelemetry.io/ebpf-profiler/times"
	"go.opentelemetry.io/ebpf-profiler/util"
)

// isPIDLive checks if a PID belongs to a live process. It will never produce a false negative but
// may produce a false positive (e.g. due to permissions) in which case an error will also be
// returned.
func isPIDLive(pid libpf.PID) (bool, error) {
	// Check first with the kill syscall which is the fastest route.
	// A kill syscall with a 0 signal is documented to still do the check
	// whether the process exists: https://linux.die.net/man/2/kill
	err := unix.Kill(int(pid), 0)
	if err == nil {
		return true, nil
	}

	var errno unix.Errno
	if errors.As(err, &errno) {
		switch errno {
		case unix.ESRCH:
			return false, nil
		case unix.EPERM:
			// It seems that in some rare cases this check can fail with
			// a permission error. Fallback to a procfs check.
		default:
			return true, err
		}
	}

	path := fmt.Sprintf("/proc/%d/maps", pid)
	_, err = os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return false, nil
	}

	return true, err
}

// assignLibcInfo updates the LibcInfo for the Interpreters on given PID.
// Caller must hold pm.mu write lock.
func (pm *ProcessManager) assignLibcInfo(pid libpf.PID, libcInfo *libc.LibcInfo) {
	if libcInfo == nil {
		return
	}

	var newLibcInfo = *libcInfo
	info, ok := pm.pidToProcessInfo[pid]
	if !ok {
		// This is guaranteed not to happen since assignLibcInfo is always called after
		// pm.updatePidInformation - but to avoid a possible panic we just return here.
		return
	} else if info.libcInfo != nil {
		if info.libcInfo.IsEqual(newLibcInfo) {
			return
		} else {
			info.libcInfo.Merge(newLibcInfo)
			newLibcInfo = *info.libcInfo
		}
	}

	info.libcInfo = &newLibcInfo

	// Update the tsdInfo to interpreters that are already attached
	for _, instance := range pm.interpreters[pid] {
		if err := instance.UpdateLibcInfo(pm.ebpf, pid, newLibcInfo); err != nil {
			log.Errorf("Failed to update PID %v LibcInfo: %v",
				pid, err)
		}
	}
}

// getLibcInfo retrieves the LibcInfo of given PID
// Caller must hold pm.mu read lock.
func (pm *ProcessManager) getLibcInfo(pid libpf.PID) *libc.LibcInfo {
	if info, ok := pm.pidToProcessInfo[pid]; ok {
		return info.libcInfo
	}
	return nil
}

// getPidInformation gets or creates the Pid information for given PID.
//
// Caller must hold pm.mu write lock.
func (pm *ProcessManager) getPidInformation(pid libpf.PID, pr process.Process,
) *processInfo {
	if info, ok := pm.pidToProcessInfo[pid]; ok {
		return info
	}

	// Insert a dummy page into the eBPF map pid_page_to_mapping_info that provides the eBPF
	// a quick way to check if we know something about this particular process.
	if err := pm.ebpf.UpdatePidPageMappingInfo(pid, dummyPrefix, 0, 0); err != nil {
		return nil
	}

	info := &processInfo{
		meta:     pr.GetProcessMeta(process.MetaConfig{IncludeEnvVars: pm.includeEnvVars}),
		libcInfo: nil,
	}
	pm.pidToProcessInfo[pid] = info
	pm.pidPageToMappingInfoSize++
	return info
}

// assignInterpreter will update the interpreters maps with given interpreter.Instance.
// Caller is responsible to hold pm.mu write lock to avoid race conditions.
func (pm *ProcessManager) assignInterpreter(pid libpf.PID, key util.OnDiskFileIdentifier,
	instance interpreter.Instance,
) {
	if _, ok := pm.interpreters[pid]; !ok {
		// This is the very first interpreter entry for this process.
		// So we need to initialize the structure first.
		pm.interpreters[pid] = make(map[util.OnDiskFileIdentifier]interpreter.Instance)
	}
	pm.interpreters[pid][key] = instance
}

// handleNewInterpreter is called to process new executable memory mappings. It uses the
// process manager to attach to the process/memory mapping if it is discovered that the
// memory mapping corresponds with an interpreter.
//
// It is important to note that this function may spawn a new goroutine in order to retry
// attaching to the interpreter, if the first attach attempt fails. In this case, `nil` will still
// be returned and thus a `nil` return value does not mean the attach was successful. It means
// that the attach was successful OR a retry is underway.
//
// The caller is responsible to hold the ProcessManager lock to avoid race conditions.
func (pm *ProcessManager) handleNewInterpreter(pr process.Process, bias libpf.Address,
	oid util.OnDiskFileIdentifier, data interpreter.Data) error {
	// The same interpreter can be found multiple times under various different
	// circumstances. Check if this is already handled.
	pid := pr.PID()
	if _, ok := pm.interpreters[pid]; ok {
		if _, ok := pm.interpreters[pid][oid]; ok {
			return nil
		}
	}
	// Slow path: Interpreter detection or attachment needed
	instance, err := data.Attach(pm.ebpf, pid, bias, pr.GetRemoteMemory())
	if err != nil {
		return fmt.Errorf("failed to attach to %v in PID %v: %w",
			data, pid, err)
	}

	log.Debugf("Attached to %v interpreter in PID %v", data, pid)
	pm.assignInterpreter(pid, oid, instance)

	if libcInfo := pm.getLibcInfo(pid); libcInfo != nil {
		err = instance.UpdateLibcInfo(pm.ebpf, pid, *libcInfo)
		if err != nil {
			log.Errorf("Failed to update PID %v LibcInfo: %v", pid, err)
		}
	}

	return nil
}

func (pm *ProcessManager) getELFInfo(pr process.Process, mapping *process.Mapping,
	elfRef *pfelf.Reference,
) elfInfo {
	key := mapping.GetOnDiskFileIdentifier()
	lastModified := pr.GetMappingFileLastModified(mapping)
	if info, ok := pm.elfInfoCache.Get(key); ok && info.lastModified == lastModified {
		// Cached data ok
		pm.elfInfoCacheHit.Add(1)
		return info
	}

	// Slow path, calculate all the data and update cache
	pm.elfInfoCacheMiss.Add(1)

	info := elfInfo{
		lastModified: lastModified,
	}

	var fileID libpf.FileID
	ef, err := elfRef.GetELF()
	if err == nil {
		fileID, err = pr.CalculateMappingFileID(mapping)
	}
	if err != nil {
		info.err = err
		// It is possible that the process has exited, and the mapping
		// file cannot be opened. Do not cache these errors.
		if !errors.Is(err, os.ErrNotExist) {
			// Cache the other errors: not an ELF, ELF corrupt, etc.
			// to reduce opening it again and again.
			pm.elfInfoCache.Add(key, info)
		}
		return info
	}

	baseName := path.Base(mapping.Path.String())
	if baseName == "/" {
		// There are circumstances where there is no filename.
		// E.g. kernel module 'bpfilter_umh' before Linux 5.9-rc1 uses
		// fork_usermode_blob() and launches process with a blob without
		// filename mapped in as the executable.
		baseName = "<anonymous-blob>"
	}
	gnuBuildID, _ := ef.GetBuildID()
	goBuildID := ""
	if ef.IsGolang() {
		goBuildID, _ = ef.GetGoBuildID()
	}

	info.mappingFile = libpf.NewFrameMappingFile(libpf.FrameMappingFileData{
		FileID:     fileID,
		FileName:   libpf.Intern(baseName),
		GnuBuildID: gnuBuildID,
		GoBuildID:  goBuildID,
	})

	info.addressMapper = ef.GetAddressMapper()
	pm.elfInfoCache.Add(key, info)

	pm.exeReporter.ReportExecutable(&reporter.ExecutableMetadata{
		MappingFile:       info.mappingFile,
		Process:           pr,
		Mapping:           mapping,
		DebuglinkFileName: ef.DebuglinkFileName(elfRef.FileName(), elfRef),
	})

	return info
}

func (pm *ProcessManager) processNewMapping(pid libpf.PID, m *Mapping) uint64 {
	mf := m.FrameMapping.Value()

	// Update the eBPF maps with information about this mapping.
	prefixes, err := lpm.CalculatePrefixList(uint64(m.Vaddr), uint64(m.Vaddr+mf.End-mf.Start))
	if err != nil {
		log.Errorf("Failed to create LPM entries for PID %d: %v", pid, err)
		return 0
	}

	added := uint64(0)
	bias := uint64(m.Vaddr - mf.Start)
	fileID := uint64(host.FileIDFromLibpf(mf.File.Value().FileID))
	for _, prefix := range prefixes {
		if err = pm.ebpf.UpdatePidPageMappingInfo(pid, prefix, fileID, bias); err != nil {
			log.Errorf("Failed to update pid_page_to_mapping_info (pid: %d, page: 0x%x/%d): %v",
				pid, prefix.Key, prefix.Length, err)
			break
		}
		added++
	}
	return added
}

func (pm *ProcessManager) processRemovedMapping(pid libpf.PID, m *Mapping) uint64 {
	mf := m.FrameMapping.Value()
	prefixes, err := lpm.CalculatePrefixList(uint64(m.Vaddr), uint64(m.Vaddr+mf.End-mf.Start))
	if err != nil {
		log.Errorf("Failed to create LPM entries for PID %d: %v", pid, err)
		return 0
	}

	deleted, err := pm.ebpf.DeletePidPageMappingInfo(pid, prefixes)
	if err != nil {
		log.Errorf("Failed to delete mappings for PID %d: %v", pid, err)
	}

	fileID := host.FileIDFromLibpf(mf.File.Value().FileID)
	pm.eim.DecRef(fileID)
	return uint64(deleted)
}

// Caller is responsible to hold pm.mu write lock to avoid race conditions.
func (pm *ProcessManager) processRemovedInterpreters(pid libpf.PID,
	interpretersValid libpf.Set[util.OnDiskFileIdentifier]) {
	if !pm.interpreterTracerEnabled {
		return
	}

	if _, ok := pm.interpreters[pid]; !ok {
		return
	}

	for key, instance := range pm.interpreters[pid] {
		if _, ok := interpretersValid[key]; ok {
			continue
		}
		if err := instance.Detach(pm.ebpf, pid); err != nil {
			log.Errorf("Failed to unload interpreter for PID %d: %v",
				pid, err)
		}
		delete(pm.interpreters[pid], key)
	}

	if len(pm.interpreters[pid]) == 0 {
		// There are no longer any mapped interpreters in the process, therefore we can
		// remove the entry.
		delete(pm.interpreters, pid)
	}
}

var errInvalidVirtualAddress = errors.New("invalid ELF virtual address")

func (pm *ProcessManager) newFrameMapping(pr process.Process, m *process.Mapping) (libpf.FrameMapping, error) {
	elfRef := pfelf.NewReference(m.Path.String(), pr)
	defer elfRef.Close()

	info := pm.getELFInfo(pr, m, elfRef)
	if info.err != nil {
		// Unable to get the information. Most likely cause is that the
		// process has exited already and the mapping file is unavailable
		// or it is not an ELF file. Ignore these errors silently.
		if !errors.Is(info.err, os.ErrNotExist) && !errors.Is(info.err, pfelf.ErrNotELF) {
			log.Debugf("Failed to get ELF info for PID %d file %v: %v",
				pr.PID(), m.Path, info.err)
		}
		return libpf.FrameMapping{}, info.err
	}

	elfSpaceVA, ok := info.addressMapper.FileOffsetToVirtualAddress(m.FileOffset)
	if !ok {
		log.Debugf("Failed to map file offset of PID %d, file %s, offset %d",
			pr.PID(), m.Path, m.FileOffset)
		return libpf.FrameMapping{}, errInvalidVirtualAddress
	}

	fileID := host.FileIDFromLibpf(info.mappingFile.Value().FileID)
	ei, err := pm.eim.AddOrIncRef(fileID, elfRef)
	if err != nil {
		return libpf.FrameMapping{}, err
	}

	pm.mu.Lock()
	pm.assignLibcInfo(pr.PID(), ei.LibcInfo)
	if ei.Data != nil {
		bias := libpf.Address(m.Vaddr - elfSpaceVA)
		pm.handleNewInterpreter(pr, bias, m.GetOnDiskFileIdentifier(), ei.Data)
	}
	pm.mu.Unlock()

	return libpf.NewFrameMapping(libpf.FrameMappingData{
		File:       info.mappingFile,
		Start:      libpf.Address(elfSpaceVA),
		End:        libpf.Address(elfSpaceVA + m.Length),
		FileOffset: m.FileOffset,
	}), nil
}

func compareMapping(a, b Mapping) int {
	aFid := host.FileIDFromLibpf(a.FrameMapping.Value().File.Value().FileID)
	bFid := host.FileIDFromLibpf(b.FrameMapping.Value().File.Value().FileID)
	if aFid != bFid {
		if aFid < bFid {
			return -1
		}
		return 1
	}
	if a.Vaddr < b.Vaddr {
		return -1
	}
	if a.Vaddr > b.Vaddr {
		return 1
	}
	return 0
}

// synchronizeMappings synchronizes executable mappings for the given PID.
// This method will be called when a PID is first encountered or when the eBPF
// code encounters an address in an executable mapping that HA has no information
// on. Therefore, executable mapping synchronization takes place lazily on-demand,
// and map/unmap operations are not precisely tracked (reduce processing load).
// This means that at any point, we may have cached stale (or miss) executable
// mappings. The expectation is that stale mappings will disappear and new
// mappings cached at the next synchronization triggered by process exit or
// unknown address encountered.
//
// TODO: Periodic synchronization of mappings for every tracked PID.
func (pm *ProcessManager) synchronizeMappings(pr process.Process,
	processMappings []process.Mapping) bool {
	pid := pr.PID()

	// Get current executable name
	exe, err := pr.GetExe()
	if err != nil && !os.IsNotExist(err) {
		// The /proc/PID/exe returns "not exists" error also in
		// the case of main thread exit. Ignore it.
		log.Warnf("Failed to get executable of process %d: %v", pid, err)
	}

	pm.mu.Lock()
	info := pm.getPidInformation(pid, pr)
	if info == nil {
		pm.mu.Unlock()
		return false
	}
	// Check if process meta needs an update
	updateProcessMeta := exe != libpf.NullString && exe != info.meta.Executable

	// Get existing info
	oldMappings := info.mappings
	newProcess := len(info.mappings) == 0
	var numInterpreters int
	if intrp, ok := pm.interpreters[pid]; ok {
		numInterpreters = len(intrp)
	}
	pm.mu.Unlock()

	// Create a lookup map for the old mappings
	mpRemove := make(map[uint64]*Mapping, len(oldMappings))
	for idx := range oldMappings {
		m := &oldMappings[idx]
		mpRemove[uint64(m.Vaddr)] = m
	}

	// Generate the list of new processmanager mappings and interpreters.
	// Reuse existing mappings if possible.
	mappings := make([]Mapping, 0, len(processMappings))
	mpAdd := make([]*Mapping, 0, len(processMappings))
	interpretersValid := make(libpf.Set[util.OnDiskFileIdentifier], numInterpreters)
	for idx := range processMappings {
		m := &processMappings[idx]
		if !m.IsExecutable() || m.IsAnonymous() {
			continue
		}

		var fm libpf.FrameMapping
		if oldm, ok := mpRemove[m.Vaddr]; ok {
			if oldm.Length == m.Length && oldm.Device == m.Device && oldm.Inode == m.Inode {
				delete(mpRemove, m.Vaddr)
				fm = oldm.FrameMapping
			}
		}
		newMapping := false
		if !fm.Valid() {
			newMapping = true
			var err error
			fm, err = pm.newFrameMapping(pr, m)
			if err != nil {
				// newFrameMapping logged the message if needed
				continue
			}
		}

		key := m.GetOnDiskFileIdentifier()
		interpretersValid[key] = libpf.Void{}

		mappings = append(mappings, Mapping{
			Vaddr:        libpf.Address(m.Vaddr),
			Length:       m.Length,
			Device:       m.Device,
			Inode:        m.Inode,
			FrameMapping: fm,
		})
		if newMapping {
			mpAdd = append(mpAdd, &mappings[len(mappings)-1])
		}
	}

	// Detach removed interpreters and remove old mappings
	numChanges := uint64(0)
	for _, m := range mpRemove {
		numChanges += pm.processRemovedMapping(pid, m)
	}
	pm.pidPageToMappingInfoSize -= numChanges
	pm.mu.Lock()
	pm.processRemovedInterpreters(pid, interpretersValid)
	pm.mu.Unlock()

	// Add new mappings
	numChanges = 0
	for _, m := range mpAdd {
		numChanges += pm.processNewMapping(pid, m)
	}
	pm.pidPageToMappingInfoSize += numChanges

	// Update metadata of the process.
	var meta process.ProcessMeta
	if updateProcessMeta {
		meta = pr.GetProcessMeta(process.MetaConfig{IncludeEnvVars: pm.includeEnvVars})
	}

	// Sort and publish the new mappings and meta
	slices.SortFunc(mappings, compareMapping)
	pm.mu.Lock()
	info = pm.getPidInformation(pid, pr)
	if info != nil {
		info.mappings = mappings
		if updateProcessMeta {
			info.meta = meta
		}
	}
	interpreters := pm.interpreters[pid]
	pm.mu.Unlock()

	// Synchronize all interpreters with updated mappings
	for _, instance := range interpreters {
		err := instance.SynchronizeMappings(pm.ebpf, pm.exeReporter, pr, processMappings)
		if err != nil {
			if alive, _ := isPIDLive(pid); alive {
				log.Errorf("Failed to handle new anonymous mapping for PID %d: %v", pid, err)
			} else {
				log.Debugf("Failed to handle new anonymous mapping for PID %d: process exited",
					pid)
			}
		}
	}

	if len(mpAdd) > 0 || len(mpRemove) > 0 || len(interpreters) > 0 {
		log.Debugf("Added %v mappings, removed %v mappings for PID %v with %d interpreters",
			len(mpAdd), len(mpRemove), pid, len(interpreters))
	}
	return newProcess
}

// processPIDExit informs the ProcessManager that a process exited and no longer will be scheduled.
// exitKTime is stored for later processing in ProcessedUntil, when traces up to this time have been
// processed. There can be a race condition if we can not clean up the references for this process
// fast enough and this particular pid is reused again by the system.
func (pm *ProcessManager) processPIDExit(pid libpf.PID) {
	exitKTime := times.GetKTime()
	log.Debugf("- PID: %v", pid)

	var err error
	defer func() {
		if err != nil {
			log.Error(err)
		}
	}()
	defer pm.ebpf.RemoveReportedPID(pid)
	pm.mu.Lock()
	defer pm.mu.Unlock()

	info, pidExists := pm.pidToProcessInfo[pid]
	if !pidExists {
		log.Debugf("Skip process exit handling for unknown PID %d", pid)
		return
	}

	// processPIDExit may be called multiple times in short succession
	// for the same PID, don't update exitKTime if we've previously recorded it.
	if _, pidExitProcessed := pm.exitEvents[pid]; !pidExitProcessed {
		pm.exitEvents[pid] = exitKTime
	} else {
		log.Debugf("Skip duplicate process exit handling for PID %d", pid)
		return
	}

	// Delete all entries we have for this particular PID from pid_page_to_mapping_info.
	deleted, err2 := pm.ebpf.DeletePidPageMappingInfo(pid, []lpm.Prefix{dummyPrefix})
	if err2 != nil {
		err = errors.Join(err, fmt.Errorf("failed to delete dummy prefix for PID %d: %v",
			pid, err2))
	}
	pm.pidPageToMappingInfoSize -= uint64(deleted)

	for idx := range info.mappings {
		pm.processRemovedMapping(pid, &info.mappings[idx])
	}
	pm.processRemovedInterpreters(pid, libpf.Set[util.OnDiskFileIdentifier]{})
}

// SynchronizeProcess triggers ProcessManager to update its internal information
// about a process. This includes process exit information as well as changed memory mappings.
func (pm *ProcessManager) SynchronizeProcess(pr process.Process) {
	pid := pr.PID()
	log.Debugf("= PID: %v", pid)

	// Abort early if process is waiting for cleanup in ProcessedUntil
	pm.mu.Lock()
	_, ok := pm.exitEvents[pid]
	pm.mu.Unlock()

	if ok {
		log.Debugf("PID %v waiting for cleanup, aborting SynchronizeProcess", pid)
		pm.ebpf.RemoveReportedPID(pid)
		return
	}

	pm.mappingStats.numProcAttempts.Add(1)
	start := time.Now()
	mappings, numParseErrors, err := pr.GetMappings()
	elapsed := time.Since(start)
	pm.mappingStats.numProcParseErrors.Add(numParseErrors)

	if err != nil {
		if os.IsPermission(err) {
			// Ignore the synchronization completely in case of permission
			// error. This implies the process is still alive, but we cannot
			// inspect it. Exiting here keeps the PID in the eBPF maps so
			// we avoid a notification flood to resynchronize.
			pm.mappingStats.errProcPerm.Add(1)
			return
		}

		if errors.Is(err, process.ErrNoMappings) {
			// When no mappings can be extracted but the process is still alive,
			// do not trigger a process exit to avoid unloading process metadata.
			// As it's likely that a future iteration can extract mappings from a
			// different thread in the process, notify eBPF to enable further notifications.
			pm.ebpf.RemoveReportedPID(pid)
			return
		}

		// All other errors imply that the process has exited.
		if os.IsNotExist(err) {
			// Since listing /proc and opening files in there later is inherently racy,
			// we expect to lose the race sometimes and thus expect to hit os.IsNotExist.
			pm.mappingStats.errProcNotExist.Add(1)
		} else if e, ok := err.(*os.PathError); ok && e.Err == syscall.ESRCH {
			// If the process exits while reading its /proc/$PID/maps, the kernel will
			// return ESRCH. Handle it as if the process did not exist.
			pm.mappingStats.errProcESRCH.Add(1)
		}
		// Clean up, and notify eBPF.
		log.Debugf("removing pid due to mappings read error: %v", err)
		pm.processPIDExit(pid)
		return
	}

	util.AtomicUpdateMaxUint32(&pm.mappingStats.maxProcParseUsec, uint32(elapsed.Microseconds()))
	pm.mappingStats.totalProcParseUsec.Add(uint32(elapsed.Microseconds()))

	if pm.synchronizeMappings(pr, mappings) {
		log.Debugf("+ PID: %v", pid)
		// TODO: Fine-grained reported_pids handling (evaluate per-PID mapping
		// synchronization based on per-PID state such as time since last
		// synchronization). Currently we only remove a PID from reported_pids
		// if it's a new process and on process exit. This limits
		// the frequency of PID mapping synchronizations to PID lifetime in
		// reported_pids (which is dictated by REPORTED_PIDS_TIMEOUT in eBPF).

		// We're immediately removing a new PID from reported_pids, to cover
		// corner cases where processes load on startup in quick-succession
		// additional code (e.g. plugins, Asterisk).
		// Also see: Unified PID Events design doc
		pm.ebpf.RemoveReportedPID(pid)
	}
}

// CleanupPIDs executes a periodic synchronization of pidToProcessInfo table with system processes.
// NOTE: Exported only for tracer.
func (pm *ProcessManager) CleanupPIDs() {
	deadPids := make([]libpf.PID, 0, 16)

	pm.mu.RLock()
	for pid := range pm.pidToProcessInfo {
		if live, _ := isPIDLive(pid); !live {
			deadPids = append(deadPids, pid)
		}
	}
	pm.mu.RUnlock()

	for _, pid := range deadPids {
		pm.processPIDExit(pid)
	}

	if len(deadPids) > 0 {
		log.Debugf("Cleaned up %d dead PIDs", len(deadPids))
	}
}

// MetaForPID returns the process metadata for given PID.
func (pm *ProcessManager) MetaForPID(pid libpf.PID) process.ProcessMeta {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if procInfo, ok := pm.pidToProcessInfo[pid]; ok {
		return procInfo.meta
	}
	return process.ProcessMeta{}
}

// findMappingForTrace locates the mapping for a given host trace.
func (pm *ProcessManager) findMappingForTrace(pid libpf.PID, fid host.FileID,
	addr libpf.Address) libpf.FrameMapping {
	var maps []Mapping

	pm.mu.RLock()
	if procInfo, ok := pm.pidToProcessInfo[pid]; ok {
		maps = procInfo.mappings
	}
	pm.mu.RUnlock()
	if maps == nil {
		return libpf.FrameMapping{}
	}

	// Binary search for the potentially matching 'maps' entry. The search
	// lambda makes 'sort.Search' return the first entry that is larger
	// than the fid/addr pair. Thus -1 is needed to get index for the first
	// entry which is equal or less than fid/addr pair.
	i := sort.Search(len(maps), func(i int) bool {
		entry := &maps[i]
		fm := entry.FrameMapping.Value()
		f := fm.File.Value()
		entryFid := host.FileIDFromLibpf(f.FileID)
		if entryFid != fid {
			return entryFid >= fid
		}
		return fm.Start >= addr
	}) - 1

	if i >= 0 {
		entry := &maps[i]
		fm := entry.FrameMapping.Value()
		f := fm.File.Value()
		entryFid := host.FileIDFromLibpf(f.FileID)
		// Validate that the candidate 'maps' entry is a true match.
		if entryFid == fid && fm.Start <= addr && addr < fm.End {
			return entry.FrameMapping
		}
	}
	return libpf.FrameMapping{}
}

func (pm *ProcessManager) ProcessedUntil(traceCaptureKTime times.KTime) {
	var err error
	defer func() {
		if err != nil {
			log.Error(err)
		}
	}()
	pm.mu.Lock()
	defer pm.mu.Unlock()

	nowKTime := times.GetKTime()
	log.Debugf("ProcessedUntil captureKT: %v latency: %v ms",
		traceCaptureKTime, (nowKTime-traceCaptureKTime)/1e6)

	for pid, pidExitKTime := range pm.exitEvents {
		if pidExitKTime > traceCaptureKTime {
			continue
		}

		log.Debugf("PID %v deleted", pid)
		delete(pm.pidToProcessInfo, pid)

		for _, instance := range pm.interpreters[pid] {
			if err2 := instance.Detach(pm.ebpf, pid); err2 != nil {
				err = errors.Join(err,
					fmt.Errorf("failed to handle interpreted process exit for PID %d: %v",
						pid, err2))
			}
		}
		delete(pm.interpreters, pid)
		delete(pm.exitEvents, pid)
		log.Debugf("PID %v exit latency %v ms", pid, (nowKTime-pidExitKTime)/1e6)
	}
}
