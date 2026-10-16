// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package jvmmemory

import (
	"errors"
	"fmt"
	"sync"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/ebpf/usdt"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// maxAttachedFiles caps the number of libjvm.so files with attached probes.
// Each container has its own file (overlayfs device).
const maxAttachedFiles = 512

// cloneKey identifies a program clone: one per eBPF program and USDT spec.
type cloneKey struct {
	funcName string
	specID   uint32
}

// programClone is an instance of an eBPF program whose usdt_spec_id constant
// is the id of a USDT spec. It is loaded by ebpf-manager's CloneProgram,
// which also attaches it to the first site using that spec: the anchor. The
// other sites using that spec are attached with AddHook, which reuses the
// loaded program. The clone is released when no site uses it anymore.
type programClone struct {
	key    cloneKey
	anchor *manager.Probe
	// refs is the number of attached sites using the clone, the anchor
	// included while it is attached.
	refs int
}

// attachedSite is an attached probe site.
type attachedSite struct {
	probe *manager.Probe
	// clone is the program clone of the site, nil for the programs that do
	// not read the probe arguments (attached with the base program).
	clone *programClone
}

// isAnchor returns whether the probe of s is the anchor of its clone.
func (s *attachedSite) isAnchor() bool {
	return s.clone != nil && s.clone.anchor == s.probe
}

// usdtAttacher attaches the eBPF programs to the HotSpot probe sites of the
// libjvm.so files. Its attach and detach methods are the activation and
// deactivation callbacks of the file registry. It is safe for concurrent use.
type usdtAttacher struct {
	mgr         *manager.Manager
	specMap     *maps.GenericMap[uint32, usdt.Spec]
	hostMachine safeelf.Machine

	mu       sync.Mutex
	registry usdt.SpecRegistry
	clones   map[cloneKey]*programClone
	files    map[utils.PathIdentifier][]*attachedSite
	fileSeq  uint64
}

func newUSDTAttacher(mgr *manager.Manager, specMap *maps.GenericMap[uint32, usdt.Spec], hostMachine safeelf.Machine) *usdtAttacher {
	return &usdtAttacher{
		mgr:         mgr,
		specMap:     specMap,
		hostMachine: hostMachine,
		clones:      make(map[cloneKey]*programClone),
		files:       make(map[utils.PathIdentifier][]*attachedSite),
	}
}

// attach attaches the programs to the HotSpot probe sites of a libjvm.so.
//
// The file is parsed from an open file descriptor, whose (device, inode) must
// be the one the registry identified, and the uprobes are attached through
// that same descriptor: a file swapped by a container between the parsing and
// the attachment cannot redirect the uprobes to another file.
func (a *usdtAttacher) attach(fp utils.FilePath) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.files[fp.ID]; ok {
		return nil
	}
	if len(a.files) >= maxAttachedFiles {
		probeTelemetry.fileErrors.Inc(reasonTooMany)
		// not the fault of the file: retried at the next scan
		return retryLater(fmt.Errorf("%d libjvm.so files already attached", len(a.files)))
	}

	f, err := openLibJVM(fp)
	if err != nil {
		probeTelemetry.fileErrors.Inc(reasonOpen)
		return err
	}
	defer f.Close()

	sites, err := readHotSpotSites(f, a.hostMachine)
	if err != nil {
		switch {
		case errors.Is(err, errNoHotSpotProbes):
			probeTelemetry.filesWithoutProbes.Inc()
			log.Debugf("jvm_memory: ignoring %s: %s", fp.HostPath, err)
		case errors.Is(err, errTooManySites):
			probeTelemetry.fileErrors.Inc(reasonTooSites)
		case errors.Is(err, errForeignArch):
			probeTelemetry.fileErrors.Inc(reasonArch)
		default:
			probeTelemetry.fileErrors.Inc(reasonParse)
		}
		return fmt.Errorf("%s: %w", fp.HostPath, err)
	}

	// The kernel resolves this path in the context of system-probe, to the
	// file opened above.
	binaryPath := fmt.Sprintf("/proc/self/fd/%d", f.Fd())
	a.fileSeq++
	var attached []*attachedSite
	attachedProbes := make(map[string]bool, len(hotspotProbes))
	for i, site := range sites {
		// short UIDs: they are part of the tracefs event names on kernels
		// without perf PMU uprobes
		uid := fmt.Sprintf("j%d_%d", a.fileSeq, i)
		s, err := a.attachSite(site, binaryPath, uid)
		if err != nil {
			probeTelemetry.siteAttachErrors.Inc(site.name)
			log.Debugf("jvm_memory: attaching %s:%s at %s+%#x: %s", hotspotProvider, site.name, fp.HostPath, site.offset, err)
			continue
		}
		attached = append(attached, s)
		attachedProbes[site.name] = true
	}
	if !producesData(attachedProbes) {
		for _, s := range attached {
			a.detachSite(s)
		}
		probeTelemetry.fileErrors.Inc(reasonAttach)
		a.updateGauges()
		return fmt.Errorf("could not attach the HotSpot probes to %s: %w", fp.HostPath, utils.ErrEnvironment)
	}

	a.files[fp.ID] = attached
	a.updateGauges()
	probeTelemetry.filesAttached.Inc()
	log.Debugf("jvm_memory: attached %d/%d HotSpot probe sites to %s", len(attached), len(sites), fp.HostPath)
	return nil
}

// detach detaches the programs from the probe sites of a libjvm.so.
func (a *usdtAttacher) detach(fp utils.FilePath) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.detachFileLocked(fp.ID)
	a.updateGauges()
	return nil
}

// close detaches all the probe sites.
func (a *usdtAttacher) close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for id := range a.files {
		a.detachFileLocked(id)
	}
	a.updateGauges()
}

// detachFileLocked detaches the probe sites of a file, and forgets it. a.mu
// must be held.
func (a *usdtAttacher) detachFileLocked(id utils.PathIdentifier) {
	for _, s := range a.files[id] {
		a.detachSite(s)
	}
	delete(a.files, id)
}

// stats returns the number of attached files and of USDT specs in use.
func (a *usdtAttacher) stats() (files int, specs int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.files), a.registry.Len()
}

func (a *usdtAttacher) updateGauges() {
	probeTelemetry.attachedFiles.Set(float64(len(a.files)))
	probeTelemetry.specs.Set(float64(a.registry.Len()))
}

func newSiteProbe(funcName, uid, binaryPath string, offset uint64) *manager.Probe {
	return &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: uid, EBPFFuncName: funcName},
		BinaryPath:              binaryPath,
		UprobeOffset:            offset,
		// The probes share the program spec of the program they are attached
		// with: keep its instructions, which CloneProgram copies.
		KeepProgramSpec: true,
	}
}

// attachSite attaches the program of a site, with the program clone of its
// USDT spec when the program reads the probe arguments.
func (a *usdtAttacher) attachSite(site hotspotSite, binaryPath, uid string) (*attachedSite, error) {
	funcName := site.probe.funcName
	if site.probe.minArgs == 0 {
		// The base program: its usdt_spec_id constant is 0, an id that is
		// never allocated.
		p := newSiteProbe(funcName, uid, binaryPath, site.offset)
		if err := a.addHook("", p); err != nil {
			return nil, err
		}
		return &attachedSite{probe: p}, nil
	}

	id, err := a.acquireSpec(site.spec)
	if err != nil {
		return nil, err
	}
	s, err := a.attachSiteWithSpec(site, binaryPath, uid, id)
	if err != nil {
		a.releaseSpec(id)
		return nil, err
	}
	return s, nil
}

// attachSiteWithSpec attaches the program of a site with the program clone
// of the acquired USDT spec id. The caller releases id on error.
func (a *usdtAttacher) attachSiteWithSpec(site hotspotSite, binaryPath, uid string, id uint32) (*attachedSite, error) {
	funcName := site.probe.funcName
	key := cloneKey{funcName: funcName, specID: id}
	if clone, ok := a.clones[key]; ok {
		p := newSiteProbe(funcName, uid, binaryPath, site.offset)
		if err := a.addHook(clone.anchor.UID, p); err != nil {
			return nil, err
		}
		clone.refs++
		return &attachedSite{probe: p, clone: clone}, nil
	}

	// First site of this program and spec: load a clone of the program
	// reading the arguments with this spec, attached to the site.
	anchor := newSiteProbe(funcName, fmt.Sprintf("s%d", id), binaryPath, site.offset)
	editors := []manager.ConstantEditor{{Name: usdt.SpecIDConstant, Value: uint64(id)}}
	if err := a.mgr.CloneProgram("", anchor, editors, nil); err != nil {
		return nil, fmt.Errorf("cloning %s for USDT spec %d: %w", funcName, id, err)
	}
	clone := &programClone{key: key, anchor: anchor, refs: 1}
	// On an attach error not reported by CloneProgram (see notRunningError),
	// the probe is reset and its program is nil.
	if err := notRunningError(anchor); err != nil {
		a.releaseClone(clone)
		return nil, err
	}
	ddebpf.AddNameMappingsForProgram(anchor.Program(), funcName, moduleName)
	a.clones[key] = clone
	return &attachedSite{probe: anchor, clone: clone}, nil
}

// notRunningError returns the attach error of p if it is not running, nil
// otherwise. It is needed because Probe.Attach does not report the ENOENT and
// EINVAL errors.
func notRunningError(p *manager.Probe) error {
	if p.IsRunning() {
		return nil
	}
	err := p.GetLastError() //nolint:forbidigo // ebpf-manager probe error, not the Windows API
	return fmt.Errorf("attaching %s: %v", p.EBPFFuncName, err)
}

// addHook attaches p with the program of the probe identified by
// (uid, p.EBPFFuncName), uid being empty for the base program.
func (a *usdtAttacher) addHook(uid string, p *manager.Probe) error {
	if err := a.mgr.AddHook(uid, p); err != nil {
		return err
	}
	if err := notRunningError(p); err != nil {
		a.detachHook(p)
		return err
	}
	return nil
}

// detachHook detaches a probe added with AddHook or CloneProgram, removes it
// from the manager, and closes its handle on its program (for AddHook, a
// duplicate of the file descriptor of the program).
func (a *usdtAttacher) detachHook(p *manager.Probe) {
	// DetachHook resets the probe without closing its program when it is not
	// running, or when it is the last probe of its program.
	prog := p.Program()
	if err := a.mgr.DetachHook(p.ProbeIdentificationPair); err != nil {
		log.Debugf("jvm_memory: detaching %s: %s", p.ProbeIdentificationPair, err)
	}
	if prog != nil {
		_ = prog.Close()
	}
}

// detachSite detaches a probe site, and releases its program clone if no
// other site uses it.
func (a *usdtAttacher) detachSite(s *attachedSite) {
	if s.isAnchor() {
		// keep the clone loaded for the other sites using it
		if err := s.probe.Detach(); err != nil {
			log.Debugf("jvm_memory: detaching %s: %s", s.probe.ProbeIdentificationPair, err)
		}
	} else {
		a.detachHook(s.probe)
	}

	if s.clone == nil {
		return
	}
	s.clone.refs--
	if s.clone.refs == 0 {
		a.releaseClone(s.clone)
	}
	a.releaseSpec(s.clone.key.specID)
}

// releaseClone detaches and unloads a program clone.
func (a *usdtAttacher) releaseClone(c *programClone) {
	progID := c.anchor.ID()
	if c.anchor.IsRunning() {
		_ = c.anchor.Detach()
	}
	// Detach keeps the program of the anchor: detachHook closes it.
	a.detachHook(c.anchor)
	ddebpf.RemoveProgramID(progID, moduleName)
	delete(a.clones, c.key)
}

// acquireSpec takes a reference on spec and returns its id, storing spec in
// the usdt_specs map when the id was just allocated. It is balanced by
// releaseSpec.
func (a *usdtAttacher) acquireSpec(spec usdt.Spec) (uint32, error) {
	id, isNew, err := a.registry.Acquire(spec)
	if err != nil {
		return 0, err
	}
	if isNew {
		if err := a.specMap.Put(&id, &spec); err != nil {
			a.releaseSpec(id)
			return 0, fmt.Errorf("storing USDT spec %d: %w", id, err)
		}
	}
	return id, nil
}

func (a *usdtAttacher) releaseSpec(id uint32) {
	if _, err := a.registry.Release(id); err != nil {
		log.Warnf("jvm_memory: %s", err)
	}
}
