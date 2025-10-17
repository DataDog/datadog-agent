// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// GetBoundSockets returns the list of bound sockets for a given process
func (bss *BoundSocketSnapshotter) GetBoundSockets(p *process.Process) ([]model.SnapshottedBoundSocket, error) {

	boundSockets := []model.SnapshottedBoundSocket{}

	// list all the file descriptors opened by the process
	FDs, err := p.OpenFiles()
	if err != nil {
		seclog.Warnf("error while listing files (pid: %v): %s", p.Pid, err)
		return nil, err
	}

	// sockets have the following pattern "socket:[inode]"
	var sockets []uint64
	for _, fd := range FDs {
		if strings.HasPrefix(fd.Path, "socket:[") {
			sock, err := strconv.Atoi(strings.TrimPrefix(fd.Path[:len(fd.Path)-1], "socket:["))
			if err != nil {
				seclog.Warnf("error while parsing socket inode (pid: %v): %s", p.Pid, err)
				continue
			}
			if sock < 0 {
				continue
			}
			sockets = append(sockets, uint64(sock))
		}
	}

	link, err := os.Readlink(kernel.HostProc(fmt.Sprintf("%d", p.Pid), "ns/net"))
	if err != nil {
		return nil, err
	}
	// link should be in for of: net:[4026542294]
	if !strings.HasPrefix(link, "net:[") {
		return nil, fmt.Errorf("failed to retrieve network namespace, net ns malformated: (%s) err: %v", link, err)
	}

	link = strings.TrimPrefix(link, "net:[")
	link = strings.TrimSuffix(link, "]")

	ns, err := strconv.ParseUint(link, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve network namespace, net ns malformated: (%s) err: %v", link, err)
	}

	var cacheEntry *netNsCacheEntry

	cacheEntry, ok := bss.netNsCache[ns]
	if !ok || cacheEntry == nil {
		// cache miss, we need to snapshot the sockets of the network namespace
		cacheEntry = &netNsCacheEntry{}
		tcp, err := parseNetIP(p.Pid, "net/tcp")
		if err != nil {
			seclog.Debugf("couldn't snapshot TCP sockets: %v", err)
		}
		udp, err := parseNetIP(p.Pid, "net/udp")
		if err != nil {
			seclog.Debugf("couldn't snapshot UDP sockets: %v", err)
		}
		cacheEntry.TCP = tcp
		cacheEntry.UDP = udp
		if ipv6exists() {
			// looking for AF_INET6 sockets
			tcp6, err := parseNetIP(p.Pid, "net/tcp6")
			if err != nil {
				seclog.Debugf("couldn't snapshot TCP6 sockets: %v", err)
			}
			udp6, err := parseNetIP(p.Pid, "net/udp6")
			if err != nil {
				seclog.Debugf("couldn't snapshot UDP6 sockets: %v", err)
			}
			cacheEntry.TCP6 = tcp6
			cacheEntry.UDP6 = udp6
		}
		bss.netNsCache[ns] = cacheEntry
	}

	// searching for socket inode
	for _, s := range sockets {
		for _, sock := range cacheEntry.TCP {
			if sock.inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.ip, Port: sock.port, Family: unix.AF_INET, Protocol: unix.IPPROTO_TCP})
				break
			}
		}
		for _, sock := range cacheEntry.UDP {
			if sock.inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.ip, Port: sock.port, Family: unix.AF_INET, Protocol: unix.IPPROTO_UDP})
				break
			}
		}
		for _, sock := range cacheEntry.TCP6 {
			if sock.inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.ip, Port: sock.port, Family: unix.AF_INET6, Protocol: unix.IPPROTO_TCP})
				break
			}
		}
		for _, sock := range cacheEntry.UDP6 {
			if sock.inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.ip, Port: sock.port, Family: unix.AF_INET6, Protocol: unix.IPPROTO_UDP})
				break
			}
		}
		// not necessary found here, can be also another kind of socket (AF_UNIX, AF_NETLINK, etc)
	}

	return boundSockets, nil
}

// NewBoundSocketSnapshotter creates a new BoundSocketSnapshotter instance
func NewBoundSocketSnapshotter() *BoundSocketSnapshotter {
	return &BoundSocketSnapshotter{
		netNsCache: make(map[uint64]*netNsCacheEntry),
	}
}

// BoundSocketSnapshotter is used to snapshot bound sockets of a process
type BoundSocketSnapshotter struct {
	netNsCache map[uint64]*netNsCacheEntry
}

type netNsCacheEntry struct {
	TCP  []netIPEntry
	UDP  []netIPEntry
	TCP6 []netIPEntry
	UDP6 []netIPEntry
}

var ipv6exists = sync.OnceValue(func() bool {
	// the existence of this path is gated on the kernel module being loaded
	// we can expect that if this path exists for the current process,
	// then it will exist for all other processes
	const ipv6Path = "/proc/self/net/tcp6"
	_, err := os.Stat("/proc/self/net/tcp6")
	if err != nil {
		if !os.IsNotExist(err) {
			seclog.Warnf("error while checking for %s: %s", ipv6Path, err)
		}
		return false
	}
	return true
})

type netIPEntry struct {
	ip    net.IP
	port  uint16
	inode uint64
}

func parseNetIP(pid int32, suffix string) ([]netIPEntry, error) {
	path := kernel.HostProc(fmt.Sprintf("%d", pid), suffix)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseNetIPFromReader(f)
}

func parseNetIPFromReader(r io.Reader) ([]netIPEntry, error) {
	var netTCP []netIPEntry

	const readLimit = 4294967296 // Byte -> 4 GiB
	lr := io.LimitReader(r, readLimit)
	s := bufio.NewScanner(lr)
	s.Scan() // skip first line with headers
	for s.Scan() {
		line := s.Bytes()
		localF := fieldN(line, 1)
		ipF, portF, found := strings.Cut(localF, ":")
		if !found {
			return nil, fmt.Errorf("unexpected format for local address: %s", localF)
		}

		ip, err := parseIP(ipF)
		if err != nil {
			return nil, err
		}

		port64, err := strconv.ParseUint(portF, 16, 16)
		if err != nil {
			return nil, err
		}
		port := uint16(port64)

		inodeF := fieldN(line, 9)
		inode, err := strconv.ParseUint(inodeF, 10, 64)
		if err != nil {
			return nil, err
		}

		netTCP = append(netTCP, netIPEntry{
			ip:    ip,
			port:  port,
			inode: inode,
		})
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return netTCP, nil
}

func fieldN(text []byte, n int) string {
	var word []byte
	n++ // to run one last time
	for ; n != 0; n-- {
		// eat first whitespaces
		for len(text) != 0 && (text[0] == ' ' || text[0] == '\t') {
			text = text[1:]
		}

		nextSpace := bytes.IndexAny(text, " \t")
		if nextSpace < 0 {
			return ""
		}

		word = text[:nextSpace]
		text = text[nextSpace:]
	}
	return string(word)
}

func parseIP(hexIP string) (net.IP, error) {
	byteIP, err := hex.DecodeString(hexIP)
	if err != nil {
		return nil, fmt.Errorf("cannot parse socket field in %q: %w", hexIP, err)
	}
	switch len(byteIP) {
	case 4:
		return net.IPv4(byteIP[3], byteIP[2], byteIP[1], byteIP[0]), nil
	case 16:
		i := net.IP{
			byteIP[3], byteIP[2], byteIP[1], byteIP[0],
			byteIP[7], byteIP[6], byteIP[5], byteIP[4],
			byteIP[11], byteIP[10], byteIP[9], byteIP[8],
			byteIP[15], byteIP[14], byteIP[13], byteIP[12],
		}
		return i, nil
	default:
		return nil, fmt.Errorf("unable to parse IP %s: %v", hexIP, nil)
	}
}
