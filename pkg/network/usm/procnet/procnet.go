// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package procnet is responsible for scraping procFS and returning a list of
// all existing sockets, including information such as their local/remote
// addresses, PIDs and file descriptor numbers.
//
// The main motivation is to provide a way to fetch socket information for
// connections that were created prior system-probe startup, so we can use this
// data to "pre-populate" some of our eBPF maps.
package procnet

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TCPConnection encapsulates information for a TCP connection.
// This information is obtained from the /proc/<PID>/net/{tcp,tcp6} files
// along with /proc/<PID>/fds directories.
type TCPConnection struct {
	// Sourced from /proc/<PID>/net/{tcp,tcp6}
	Laddr netip.Addr
	Raddr netip.Addr
	Lport uint16
	Rport uint16
	State int

	// Sourced from /proc/<PID>/fd
	PID uint32
	FD  uint32
	// Sourced from /proc/<PID>/ns
	NetNS uint32
}

// GetTCPConnections returns a list of all existing TCP connections
func GetTCPConnections() []TCPConnection {
	var (
		procRoot    = kernel.ProcFSRoot()
		seenNS      = make(map[uint32]struct{})
		connByInode = make(map[int]TCPConnection, 100)
		result      = make([]TCPConnection, 100)
	)

	_ = kernel.WithAllProcs(procRoot, func(pid int) error {
		ino, err := kernel.GetNetNsInoFromPid(procRoot, pid)
		if err != nil {
			return nil
		}

		if _, ok := seenNS[ino]; !ok {
			populateIndex(connByInode, ino, filepath.Join(procRoot, fmt.Sprintf("%d", pid), "net", "tcp"))
			populateIndex(connByInode, ino, filepath.Join(procRoot, fmt.Sprintf("%d", pid), "net", "tcp6"))
		}
		seenNS[ino] = struct{}{}

		result = matchFDWithSocket(procRoot, pid, connByInode, result)
		return nil
	})

	return result
}

// populateIndex builds an index of TCP connection data by inode by
// reading /proc/net/{tcp,tcp6} files
func populateIndex(connByInode map[int]TCPConnection, ino uint32, file string) {
	scanner, err := newScanner(file)
	if err != nil {
		return
	}
	defer scanner.Close()

	for {
		entry, ok := scanner.Next()
		if !ok {
			break
		}

		laddr, lport := entry.LocalAddress()
		raddr, rport := entry.RemoteAddress()
		connByInode[entry.Inode()] = TCPConnection{
			Laddr: laddr,
			Raddr: raddr,
			Lport: lport,
			Rport: rport,
			State: entry.ConnectionState(),
			NetNS: ino,
		}
	}
}

// matchFDWithSocket checks every file descriptor of a given PID and try to
// match it against socket data using the inode number.
// In case there is a match, we augment TCPConnection data with PID and FD
// information and add that to the `conns` slice.
// Note that the resulting `conns` slice can actually be bigger than the
// original `connsByInode` map size because one TCP socket can potentially "map"
// to multiple (PID, FD) pairs (eg. forked processes etc).
func matchFDWithSocket(procRoot string, pid int, connByInode map[int]TCPConnection, conns []TCPConnection) []TCPConnection {
	fdsPath := filepath.Join(procRoot, fmt.Sprintf("%d", pid), "fd")
	fdsDir, err := os.Open(fdsPath)
	if err != nil {
		return conns
	}
	defer fdsDir.Close()

	fds, err := fdsDir.Readdirnames(-1)
	if err != nil {
		return conns
	}

	for _, fd := range fds {
		info, err := os.Stat(filepath.Join(fdsPath, fd))
		if err != nil {
			continue
		}

		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}

		conn, ok := connByInode[int(stat.Ino)]
		if !ok {
			continue
		}

		fdNum, err := strconv.Atoi(fd)
		if err != nil {
			continue
		}

		conn.PID = uint32(pid)
		conn.FD = uint32(fdNum)
		conns = append(conns, conn)
	}

	return conns
}
