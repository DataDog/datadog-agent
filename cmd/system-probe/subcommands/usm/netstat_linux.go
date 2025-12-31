// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	sysconfigcomponent "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/network/usm/procnet"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	// /proc/net/* file field indices
	procNetFieldLocalAddr  = 1
	procNetFieldRemoteAddr = 2
	procNetFieldState      = 3
	procNetFieldInode      = 9
	procNetMinFields       = 10

	// IPv4 and IPv6 hex address lengths
	ipv4HexLength = 8
	ipv6HexLength = 32
)

// TCP connection states (from include/net/tcp_states.h)
const (
	tcpStateEstablished = iota + 1
	tcpStateSynSent
	tcpStateSynRecv
	tcpStateFinWait1
	tcpStateFinWait2
	tcpStateTimeWait
	tcpStateClose
	tcpStateCloseWait
	tcpStateLastAck
	tcpStateListen
	tcpStateClosing
)

// makeNetstatCommand returns the "usm netstat" cobra command
func makeNetstatCommand(globalParams *command.GlobalParams) *cobra.Command {
	var showTCP bool
	var showUDP bool

	cmd := makeOneShotCommand(
		globalParams,
		"netstat",
		"Show network connections similar to netstat -antpu",
		func(_ sysconfigcomponent.Component, _ *command.GlobalParams) error {
			return runNetstat(showTCP, showUDP)
		},
	)

	cmd.Flags().BoolVarP(&showTCP, "tcp", "t", true, "Show TCP connections")
	cmd.Flags().BoolVarP(&showUDP, "udp", "u", true, "Show UDP connections")

	return cmd
}

// NetConnection represents a network connection
type NetConnection struct {
	Protocol    string
	LocalAddr   string
	LocalPort   uint16
	RemoteAddr  string
	RemotePort  uint16
	State       string
	Inode       uint64
	PID         int32
	ProcessName string
}

func runNetstat(showTCP, showUDP bool) error {
	var connections []*NetConnection

	// Read TCP connections using procnet package (provides robust parsing and PID/FD mapping)
	if showTCP {
		tcpConns := procnet.GetTCPConnections()
		for _, conn := range tcpConns {
			// Skip connections with invalid addresses (zero netip.Addr values)
			// This is a safeguard - invalid addresses are not valid connections
			if !conn.Laddr.IsValid() || !conn.Raddr.IsValid() {
				continue
			}

			// Determine protocol based on IP version
			protocol := "tcp"
			if conn.Laddr.Is6() {
				protocol = "tcp6"
			}

			connections = append(connections, &NetConnection{
				Protocol:    protocol,
				LocalAddr:   conn.Laddr.String(),
				LocalPort:   conn.Lport,
				RemoteAddr:  conn.Raddr.String(),
				RemotePort:  conn.Rport,
				State:       tcpStateToString(conn.State),
				PID:         int32(conn.PID),
				ProcessName: getProcessName(int32(conn.PID)),
			})
		}
	}

	// Read UDP connections (manual parsing required - procnet package doesn't expose UDP support yet)
	if showUDP {
		udpConns, err := readProcNetUDP("/proc/net/udp", "udp")
		if err == nil {
			connections = append(connections, udpConns...)
		}
		udp6Conns, err := readProcNetUDP("/proc/net/udp6", "udp6")
		if err == nil {
			connections = append(connections, udp6Conns...)
		}

		// Map inodes to processes for UDP connections
		inodeToPID := mapInodestoProcesses()
		for _, conn := range connections {
			if conn.Protocol == "udp" || conn.Protocol == "udp6" {
				if pid, ok := inodeToPID[conn.Inode]; ok {
					conn.PID = pid
					conn.ProcessName = getProcessName(pid)
				}
			}
		}
	}

	// Sort by protocol, then local port
	sort.Slice(connections, func(i, j int) bool {
		if connections[i].Protocol != connections[j].Protocol {
			return connections[i].Protocol < connections[j].Protocol
		}
		return connections[i].LocalPort < connections[j].LocalPort
	})

	// Print output
	fmt.Println("Proto | Local Address           | Foreign Address         | State       | PID/Program")
	fmt.Println("------|-------------------------|-------------------------|-------------|------------------")
	for _, conn := range connections {
		localAddr := fmt.Sprintf("%s:%d", conn.LocalAddr, conn.LocalPort)
		remoteAddr := fmt.Sprintf("%s:%d", conn.RemoteAddr, conn.RemotePort)
		pidProgram := "-"
		if conn.PID > 0 {
			pidProgram = fmt.Sprintf("%d/%s", conn.PID, conn.ProcessName)
		}
		fmt.Printf("%-5s | %-23s | %-23s | %-11s | %s\n",
			conn.Protocol, localAddr, remoteAddr, conn.State, pidProgram)
	}

	return nil
}

// tcpStateToString converts TCP state int to readable string
func tcpStateToString(state int) string {
	states := map[int]string{
		tcpStateEstablished: "ESTABLISHED",
		tcpStateSynSent:     "SYN_SENT",
		tcpStateSynRecv:     "SYN_RECV",
		tcpStateFinWait1:    "FIN_WAIT1",
		tcpStateFinWait2:    "FIN_WAIT2",
		tcpStateTimeWait:    "TIME_WAIT",
		tcpStateClose:       "CLOSE",
		tcpStateCloseWait:   "CLOSE_WAIT",
		tcpStateLastAck:     "LAST_ACK",
		tcpStateListen:      "LISTEN",
		tcpStateClosing:     "CLOSING",
	}
	if s, ok := states[state]; ok {
		return s
	}
	return fmt.Sprintf("STATE_%d", state)
}

// readProcNetUDP reads UDP connections from /proc/net files
func readProcNetUDP(path, protocol string) ([]*NetConnection, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var connections []*NetConnection
	scanner := bufio.NewScanner(file)

	// Skip header line
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < procNetMinFields {
			continue
		}

		// Parse local address and port
		localAddrPort := strings.Split(fields[procNetFieldLocalAddr], ":")
		if len(localAddrPort) != 2 {
			continue
		}
		localAddr := parseHexIP(localAddrPort[0])
		localPort := parseHexPort(localAddrPort[1])

		// Parse remote address and port
		remoteAddrPort := strings.Split(fields[procNetFieldRemoteAddr], ":")
		if len(remoteAddrPort) != 2 {
			continue
		}
		remoteAddr := parseHexIP(remoteAddrPort[0])
		remotePort := parseHexPort(remoteAddrPort[1])

		// Parse inode (UDP doesn't have meaningful connection states like TCP)
		inode, _ := strconv.ParseUint(fields[procNetFieldInode], 10, 64)

		connections = append(connections, &NetConnection{
			Protocol:   protocol,
			LocalAddr:  localAddr,
			LocalPort:  localPort,
			RemoteAddr: remoteAddr,
			RemotePort: remotePort,
			State:      "-",
			Inode:      inode,
		})
	}

	return connections, scanner.Err()
}

// parseHexIP converts hex IP address to dotted notation
func parseHexIP(hexIP string) string {
	if len(hexIP) == ipv4HexLength {
		// IPv4 - little endian byte order
		a, _ := strconv.ParseUint(hexIP[6:8], 16, 8)
		b, _ := strconv.ParseUint(hexIP[4:6], 16, 8)
		c, _ := strconv.ParseUint(hexIP[2:4], 16, 8)
		d, _ := strconv.ParseUint(hexIP[0:2], 16, 8)
		return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
	} else if len(hexIP) == ipv6HexLength {
		// IPv6 - simplified, just return first segment for now
		return fmt.Sprintf("[%s]", hexIP[:8])
	}
	return hexIP
}

// parseHexPort converts hex port to uint16
func parseHexPort(hexPort string) uint16 {
	port, _ := strconv.ParseUint(hexPort, 16, 16)
	return uint16(port)
}

// mapInodestoProcesses maps socket inodes to PIDs
// Optimized to iterate processes first, then their FDs (instead of globbing all FDs at once)
func mapInodestoProcesses() map[uint64]int32 {
	inodeMap := make(map[uint64]int32)

	// Get all process directories
	procDirs, err := filepath.Glob(kernel.HostProc("[0-9]*"))
	if err != nil {
		return inodeMap
	}

	for _, procDir := range procDirs {
		// Extract PID from directory name
		pidStr := filepath.Base(procDir)
		pid, err := strconv.ParseInt(pidStr, 10, 32)
		if err != nil {
			continue
		}

		// Read FDs for this process
		fdDir := filepath.Join(procDir, "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			fdPath := filepath.Join(fdDir, fd.Name())
			target, err := os.Readlink(fdPath)
			if err != nil {
				continue
			}

			// Check if it's a socket and extract inode
			if inodeStr, ok := strings.CutPrefix(target, "socket:["); ok {
				inodeStr = strings.TrimSuffix(inodeStr, "]")
				inode, err := strconv.ParseUint(inodeStr, 10, 64)
				if err != nil {
					continue
				}
				inodeMap[inode] = int32(pid)
			}
		}
	}

	return inodeMap
}

// getProcessName reads the process name from /proc/PID/comm
func getProcessName(pid int32) string {
	commPath := kernel.HostProc(strconv.Itoa(int(pid)), "comm")
	data, err := os.ReadFile(commPath)
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(data))
}
