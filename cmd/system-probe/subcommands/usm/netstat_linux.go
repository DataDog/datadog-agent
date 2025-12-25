// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

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
	var showListening bool

	cmd := makeOneShotCommand(
		globalParams,
		"netstat",
		"Show network connections similar to netstat -antpu",
		func(sysprobeconfig sysconfigcomponent.Component, params *command.GlobalParams) error {
			return runNetstat(showTCP, showUDP, showListening)
		},
	)

	cmd.Flags().BoolVarP(&showTCP, "tcp", "t", true, "Show TCP connections")
	cmd.Flags().BoolVarP(&showUDP, "udp", "u", true, "Show UDP connections")
	cmd.Flags().BoolVarP(&showListening, "listening", "l", false, "Show only listening sockets")

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

func runNetstat(showTCP, showUDP, showListening bool) error {
	var connections []*NetConnection

	// Read TCP connections
	if showTCP {
		tcpConns, err := readProcNet("/proc/net/tcp", "tcp")
		if err == nil {
			connections = append(connections, tcpConns...)
		}
		tcp6Conns, err := readProcNet("/proc/net/tcp6", "tcp6")
		if err == nil {
			connections = append(connections, tcp6Conns...)
		}
	}

	// Read UDP connections
	if showUDP {
		udpConns, err := readProcNet("/proc/net/udp", "udp")
		if err == nil {
			connections = append(connections, udpConns...)
		}
		udp6Conns, err := readProcNet("/proc/net/udp6", "udp6")
		if err == nil {
			connections = append(connections, udp6Conns...)
		}
	}

	// Map inodes to processes
	inodeToPID := mapInodestoProcesses()
	for _, conn := range connections {
		if pid, ok := inodeToPID[conn.Inode]; ok {
			conn.PID = pid
			conn.ProcessName = getProcessName(pid)
		}
	}

	// Filter listening if requested
	if showListening {
		filtered := make([]*NetConnection, 0)
		for _, conn := range connections {
			if conn.State == "LISTEN" || conn.Protocol == "udp" || conn.Protocol == "udp6" {
				filtered = append(filtered, conn)
			}
		}
		connections = filtered
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

// readProcNet reads connections from /proc/net files
func readProcNet(path, protocol string) ([]*NetConnection, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var connections []*NetConnection
	scanner := bufio.NewScanner(file)

	// Skip header line
	if scanner.Scan() {
		// header
	}

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

		// Parse state
		state := parseTCPState(fields[procNetFieldState])

		// Parse inode
		inode, _ := strconv.ParseUint(fields[procNetFieldInode], 10, 64)

		connections = append(connections, &NetConnection{
			Protocol:   protocol,
			LocalAddr:  localAddr,
			LocalPort:  localPort,
			RemoteAddr: remoteAddr,
			RemotePort: remotePort,
			State:      state,
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

// parseTCPState converts hex state to readable string
func parseTCPState(hexState string) string {
	state, _ := strconv.ParseUint(hexState, 16, 8)
	states := map[uint64]string{
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
	return fmt.Sprintf("0x%X", state)
}

// mapInodestoProcesses maps socket inodes to PIDs
func mapInodestoProcesses() map[uint64]int32 {
	inodeMap := make(map[uint64]int32)

	// Iterate through /proc/*/fd/*
	procs, err := filepath.Glob("/proc/[0-9]*/fd/*")
	if err != nil {
		return inodeMap
	}

	for _, fdPath := range procs {
		// Read symlink target
		target, err := os.Readlink(fdPath)
		if err != nil {
			continue
		}

		// Check if it's a socket
		if strings.HasPrefix(target, "socket:[") {
			// Extract inode
			inodeStr := strings.TrimPrefix(target, "socket:[")
			inodeStr = strings.TrimSuffix(inodeStr, "]")
			inode, err := strconv.ParseUint(inodeStr, 10, 64)
			if err != nil {
				continue
			}

			// Extract PID from path
			parts := strings.Split(fdPath, "/")
			if len(parts) >= 3 {
				pidStr := parts[2]
				pid, err := strconv.ParseInt(pidStr, 10, 32)
				if err == nil {
					inodeMap[inode] = int32(pid)
				}
			}
		}
	}

	return inodeMap
}

// getProcessName reads the process name from /proc/PID/comm
func getProcessName(pid int32) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(data))
}
