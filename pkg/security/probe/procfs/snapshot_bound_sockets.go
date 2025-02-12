// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"fmt"
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
	proc, _ := procfs.NewFS(kernel.HostProc(fmt.Sprintf("%d", p.Pid)))
	if err != nil {
		seclog.Warnf("error while opening procfs (pid: %v): %s", p.Pid, err)
	}
	// looking for AF_INET sockets
	TCP, err := proc.NetTCP()
	if err != nil {
		seclog.Debugf("couldn't snapshot TCP sockets: %v", err)
	}
	UDP, err := proc.NetUDP()
	if err != nil {
		seclog.Debugf("couldn't snapshot UDP sockets: %v", err)
	}
	// looking for AF_INET6 sockets
	TCP6, err := proc.NetTCP6()
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
			if sock.Inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.LocalAddr, Port: uint16(sock.LocalPort), Family: unix.AF_INET, Protocol: unix.IPPROTO_TCP})
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
			if sock.Inode == s {
				boundSockets = append(boundSockets, model.SnapshottedBoundSocket{IP: sock.LocalAddr, Port: uint16(sock.LocalPort), Family: unix.AF_INET6, Protocol: unix.IPPROTO_TCP})
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
