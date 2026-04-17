// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package procutil

// Process collection for AIX via /proc/<pid>/psinfo.
//
// AIX exposes a binary psinfo_t structure at /proc/<pid>/psinfo (world-readable,
// 448 bytes, big-endian). This file reads that structure directly without
// shelling out or requiring elevated privileges.
//
// The struct layout is taken from /usr/include/sys/procfs.h on AIX 7.x ppc64.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// prTimestruc64 mirrors AIX timestruc64_t: { tv_sec int64; tv_nsec int32; _pad uint32 }
type prTimestruc64 struct {
	Sec  int64
	Nsec int32
	_    uint32
}

// lwpSinfo mirrors the representative LWP entry inside psinfo_t (120 bytes).
type lwpSinfo struct {
	LwpID   uint64
	Addr    uint64
	Wchan   uint64
	Flag    uint32
	Wtype   uint8
	State   int8
	Sname   byte // process state character: 'O','R','S','Z', etc.
	Nice    uint8
	Pri     int32
	Policy  uint32
	Clname  [8]byte
	Onpro   int32
	Bindpro int32
	Ptid    uint32
	_       uint32
	_       [7]uint64
}

// psinfo mirrors AIX psinfo_t from /usr/include/sys/procfs.h (448 bytes, big-endian ppc64).
type psinfo struct {
	Flag   uint32
	Flag2  uint32
	Nlwp   uint32 // number of threads
	_      uint32
	Uid    uint64
	Euid   uint64
	Gid    uint64
	Egid   uint64
	Pid    uint64
	Ppid   uint64
	Pgid   uint64
	Sid    uint64
	Ttydev uint64
	Addr   uint64
	Size   uint64        // virtual memory size in pages
	Rssize uint64        // resident set size in pages
	Start  prTimestruc64 // process start time
	Time   prTimestruc64 // combined user+system CPU time
	Cid    uint16
	_      uint16
	Argc   uint32
	Argv   uint64
	Envp   uint64
	Fname  [16]byte // executable name, null-terminated
	Psargs [80]byte // process args, space-separated, null-terminated
	_      [8]uint64
	Lwp    lwpSinfo // representative LWP
}

func readPsinfo(pid int32) (*psinfo, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/psinfo", pid))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var psi psinfo
	if err := binary.Read(f, binary.BigEndian, &psi); err != nil {
		return nil, err
	}
	return &psi, nil
}

func nullTermBytes(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func listPIDs() ([]int32, error) {
	dir, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	pids := make([]int32, 0, len(names))
	for _, name := range names {
		pid, err := strconv.ParseInt(name, 10, 32)
		if err != nil {
			continue // skip non-numeric entries (e.g. "net", "sys")
		}
		pids = append(pids, int32(pid))
	}
	return pids, nil
}

func psinfoToProcess(psi *psinfo, pid int32) *Process {
	name := nullTermBytes(psi.Fname[:])
	args := nullTermBytes(psi.Psargs[:])

	var cmdline []string
	if args != "" {
		cmdline = strings.Fields(args)
	}

	cpuSecs := float64(psi.Time.Sec) + float64(psi.Time.Nsec)/1e9

	return &Process{
		Pid:     pid,
		Ppid:    int32(psi.Ppid),
		Name:    name,
		Cmdline: cmdline,
		Uids:    []int32{int32(psi.Uid), int32(psi.Euid), int32(psi.Uid), int32(psi.Euid)},
		Gids:    []int32{int32(psi.Gid), int32(psi.Egid), int32(psi.Gid), int32(psi.Egid)},
		Stats: &Stats{
			CreateTime: psi.Start.Sec * 1000, // milliseconds since epoch
			Status:     string([]byte{psi.Lwp.Sname}),
			Nice:       int32(int8(psi.Lwp.Nice)),
			NumThreads: int32(psi.Nlwp),
			CPUTime: &CPUTimesStat{
				// psinfo only provides combined user+sys time; no per-category split.
				User: cpuSecs,
			},
			MemInfo: &MemoryInfoStat{
				// pr_size and pr_rssize are in kilobytes on AIX.
				RSS: psi.Rssize * 1024,
				VMS: psi.Size * 1024,
			},
			// IOStat, MemInfoEx, CtxSwitches are not available without root on AIX.
		},
	}
}

// NewProcessProbe returns a Probe for AIX.
func NewProcessProbe(options ...Option) Probe {
	p := &probe{}
	for _, option := range options {
		option(p)
	}
	return p
}

type probe struct{}

func (p *probe) Close() {}

func (p *probe) ProcessesByPID(_ time.Time, _ bool) (map[int32]*Process, error) {
	pids, err := listPIDs()
	if err != nil {
		return nil, fmt.Errorf("aix ProcessesByPID: could not list pids: %w", err)
	}

	result := make(map[int32]*Process, len(pids))
	for _, pid := range pids {
		psi, err := readPsinfo(pid)
		if err != nil {
			// Process may have exited between listPIDs and now; skip it.
			continue
		}
		result[pid] = psinfoToProcess(psi, pid)
	}
	return result, nil
}

func (p *probe) StatsForPIDs(_ []int32, _ time.Time) (map[int32]*Stats, error) {
	procs, err := p.ProcessesByPID(time.Now(), false)
	if err != nil {
		return nil, err
	}
	stats := make(map[int32]*Stats, len(procs))
	for pid, proc := range procs {
		stats[pid] = proc.Stats
	}
	return stats, nil
}

func (p *probe) StatsWithPermByPID(_ []int32) (map[int32]*StatsWithPerm, error) {
	return nil, errors.New("StatsWithPermByPID is not implemented on AIX")
}
