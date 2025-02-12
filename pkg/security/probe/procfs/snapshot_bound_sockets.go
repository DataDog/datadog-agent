// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// GetBoundSockets returns the list of bound sockets for a given process
func GetBoundSockets(p *process.Process) ([]model.SnapshottedBoundSocket, error) {

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

	// use /proc/[pid]/net/tcp,tcp6,udp,udp6 to extract the ports opened by the current process
	procPath := kernel.HostProc(fmt.Sprintf("%d", p.Pid))
	proc, _ := procfs.NewFS(procPath)
	if err != nil {
		seclog.Warnf("error while opening procfs (pid: %v): %s", p.Pid, err)
	}
	// looking for AF_INET sockets
	TCP, err := parseNetTCP(procPath, false)
	if err != nil {
		seclog.Debugf("couldn't snapshot TCP sockets: %v", err)
	}
	UDP, err := proc.NetUDP()
	if err != nil {
		seclog.Debugf("couldn't snapshot UDP sockets: %v", err)
	}
	// looking for AF_INET6 sockets
	TCP6, err := parseNetTCP(procPath, true)
	if err != nil {
		seclog.Debugf("couldn't snapshot TCP6 sockets: %v", err)
	}
	UDP6, err := proc.NetUDP6()
	if err != nil {
		seclog.Debugf("couldn't snapshot UDP6 sockets: %v", err)
	}

	// searching for socket inode
	for _, s := range sockets {
		for _, sock := range TCP {
			if sock.inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.ip, Port: sock.port, Family: unix.AF_INET, Protocol: unix.IPPROTO_TCP})
				break
			}
		}
		for _, sock := range UDP {
			if sock.Inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.LocalAddr, Port: uint16(sock.LocalPort), Family: unix.AF_INET, Protocol: unix.IPPROTO_UDP})
				break
			}
		}
		for _, sock := range TCP6 {
			if sock.inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.ip, Port: sock.port, Family: unix.AF_INET6, Protocol: unix.IPPROTO_TCP})
				break
			}
		}
		for _, sock := range UDP6 {
			if sock.Inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.LocalAddr, Port: uint16(sock.LocalPort), Family: unix.AF_INET6, Protocol: unix.IPPROTO_UDP})
				break
			}
		}
		// not necessary found here, can be also another kind of socket (AF_UNIX, AF_NETLINK, etc)
	}

	return boundSockets, nil
}

type netIPEntry struct {
	ip    net.IP
	port  uint16
	inode uint64
}

func parseNetTCP(procPath string, v6 bool) ([]netTCPEntry, error) {
	suffix := "net/tcp"
	if v6 {
		suffix = "net/tcp6"
	}

	path := filepath.Join(procPath, suffix)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseNetTCPFromReader(f)
}

func parseNetTCPFromReader(r io.Reader) ([]netTCPEntry, error) {
	var netTCP []netTCPEntry

	const readLimit = 4294967296 // Byte -> 4 GiB
	lr := io.LimitReader(r, readLimit)
	s := bufio.NewScanner(lr)
	s.Scan() // skip first line with headers
	for s.Scan() {
		fields := strings.Fields(s.Text())

		localF := fields[1]
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

		inodeF := fields[9]
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
