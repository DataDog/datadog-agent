// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/usdt"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const (
	hotspotProvider = "hotspot"

	// maxSitesPerFile caps the number of probe sites attached per file: a
	// HotSpot build has at most a handful of sites per probe.
	maxSitesPerFile = 64
)

// hotspotProbe is a HotSpot USDT probe handled by an eBPF program.
type hotspotProbe struct {
	// funcName is the eBPF program attached to the sites of the probe.
	funcName string
	// minArgs is the number of arguments read by the program. When it is not
	// 0, the program needs the USDT spec of the site.
	minArgs int
}

// hotspotProbes are the handled HotSpot probes, by USDT probe name.
var hotspotProbes = map[string]hotspotProbe{
	"gc__begin":          {funcName: "uprobe__hotspot_gc_begin", minArgs: 1},
	"gc__end":            {funcName: "uprobe__hotspot_gc_end"},
	"mem__pool__gc__end": {funcName: "uprobe__hotspot_mem_pool_gc_end", minArgs: 8},
}

// producesData returns whether a set of attached HotSpot probes, by USDT
// probe name, produces data: the memory pools, or the GC pauses, which need
// both gc__begin (creating the row of the JVM) and gc__end.
func producesData(attached map[string]bool) bool {
	return attached["mem__pool__gc__end"] || (attached["gc__begin"] && attached["gc__end"])
}

var (
	errNoHotSpotProbes = errors.New("no HotSpot USDT probe")
	errForeignArch     = errors.New("the file does not target the host architecture")
	errTooManySites    = errors.New("too many HotSpot probe sites")
)

// hotspotSite is a probe site of a libjvm.so to attach to.
type hotspotSite struct {
	name   string
	probe  hotspotProbe
	offset uint64
	spec   usdt.Spec
}

// openLibJVM opens the file identified by fp, checking that it is a regular
// file with the device and inode of fp.
func openLibJVM(fp utils.FilePath) (*os.File, error) {
	// O_NONBLOCK: never block on a FIFO swapped in place of the library
	fd, err := unix.Open(fp.HostPath, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		// Not the fault of the file identified by fp: the registry resolved
		// fp.ID with a stat that follows symbolic links, so for a library
		// replaced with a symbolic link (ELOOP) fp.ID is the one of the
		// target, which may be another file, e.g. a library of the host.
		return nil, openError("open", fp.HostPath, err)
	}
	fail := func(err error) (*os.File, error) {
		_ = unix.Close(fd)
		return nil, err
	}

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fail(openError("fstat", fp.HostPath, err))
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return fail(fmt.Errorf("%s is not a regular file", fp.HostPath))
	}
	if uint64(st.Dev) != fp.ID.Dev || st.Ino != fp.ID.Inode {
		// most likely replaced since the registry looked it up: do not block it
		return fail(retryLater(fmt.Errorf("%s changed since it was registered", fp.HostPath)))
	}
	if err := unix.SetNonblock(fd, false); err != nil {
		return fail(openError("fcntl", fp.HostPath, err))
	}
	return os.NewFile(uintptr(fd), fp.HostPath), nil
}

// openError wraps an open, fstat or fcntl error of a libjvm.so. ErrEnvironment
// keeps the file out of the block list of the registry. Resource exhaustion
// errors are also transient: errRetryLater makes the scanner retry the JVM at
// the next scan instead of never monitoring it.
func openError(op, path string, errno error) error {
	err := &os.PathError{Op: op, Path: path, Err: errno}
	switch errno {
	case unix.EMFILE, unix.ENFILE, unix.ENOMEM:
		// resource exhaustion
		return retryLater(err)
	}
	return fmt.Errorf("%w: %w", err, utils.ErrEnvironment)
}

// readHotSpotSites returns the probe sites of the handled HotSpot probes of
// a libjvm.so.
func readHotSpotSites(f *os.File, hostMachine safeelf.Machine) ([]hotspotSite, error) {
	elfFile, err := safeelf.NewFile(f)
	if err != nil {
		return nil, fmt.Errorf("parsing ELF: %w", err)
	}
	if elfFile.Machine != hostMachine {
		return nil, fmt.Errorf("%w: %s", errForeignArch, elfFile.Machine)
	}

	probes, err := usdt.ReadProbes(elfFile)
	if err != nil {
		return nil, fmt.Errorf("reading USDT probes: %w", err)
	}
	return selectHotSpotSites(probes)
}

// selectHotSpotSites keeps the sites of the handled HotSpot probes that can
// be attached.
func selectHotSpotSites(probes []usdt.Probe) ([]hotspotSite, error) {
	var sites []hotspotSite
	for _, p := range probes {
		if p.Provider != hotspotProvider {
			continue
		}
		hp, ok := hotspotProbes[p.Name]
		if !ok {
			continue
		}
		switch {
		case p.Err != nil:
			log.Debugf("jvm_memory: skipping a %s:%s site: %s", p.Provider, p.Name, p.Err)
			continue
		case p.Semaphore != 0:
			// HotSpot probes have no semaphore, and ebpf-manager cannot set
			// the reference counter of a uprobe
			log.Debugf("jvm_memory: skipping a %s:%s site with a semaphore", p.Provider, p.Name)
			continue
		case int(p.Spec.Cnt) < hp.minArgs:
			log.Debugf("jvm_memory: skipping a %s:%s site with %d arguments", p.Provider, p.Name, p.Spec.Cnt)
			continue
		}
		if len(sites) == maxSitesPerFile {
			return nil, errTooManySites
		}
		sites = append(sites, hotspotSite{name: p.Name, probe: hp, offset: p.FileOffset, spec: p.Spec})
	}
	if len(sites) == 0 {
		return nil, errNoHotSpotProbes
	}
	return sites, nil
}
