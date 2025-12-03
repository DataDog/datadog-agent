// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package boundport provides utilies for getting bound port information
package boundport

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	componentos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

var (
	ssUserRegex             = regexp.MustCompile(`(\("(?P<Process>[^"]+)",pid=(?P<PID>\d+),fd=\d+\),?)`)
	ssUserRegexProcessIdx   = ssUserRegex.SubexpIndex("Process")
	ssUserRegexPidIdx       = ssUserRegex.SubexpIndex("PID")
	hostPortRegex           = regexp.MustCompile(`(?P<Address>.+):(?P<Port>\d+)`)
	hostPortRegexAddressIdx = hostPortRegex.SubexpIndex("Address")
	hostPortRegexPortIdx    = hostPortRegex.SubexpIndex("Port")
)

// BoundPort represents a port that is bound to a process
type BoundPort interface {
	LocalAddress() string
	LocalPort() int
	Transport() string
	Process() string
	PID() int
}

type boundPort struct {
	localAddress string
	localPort    int
	transport    string
	processName  string
	pid          int
}

func (b *boundPort) LocalAddress() string {
	return b.localAddress
}

func (b *boundPort) LocalPort() int {
	return b.localPort
}

func (b *boundPort) Transport() string {
	return b.transport
}

func (b *boundPort) Process() string {
	return b.processName
}

func (b *boundPort) PID() int {
	return b.pid
}

func newBoundPort(localAddress string, localPort int, transport, processName string, pid int) *boundPort {
	return &boundPort{
		localAddress: localAddress,
		localPort:    localPort,
		transport:    transport,
		processName:  processName,
		pid:          pid,
	}
}

// BoundPorts returns a list of ports that are bound on the host
func BoundPorts(host *components.RemoteHost) ([]BoundPort, error) {
	os := host.OSFamily
	if os == componentos.LinuxFamily {
		return boundPortsUnix(host)
	} else if os == componentos.WindowsFamily {
		return boundPortsWindows(host)
	}
	return nil, fmt.Errorf("unsupported OS type: %v", os)
}

func parseHostPort(address string) (string, int, error) {
	matches := hostPortRegex.FindStringSubmatch(address)
	if len(matches) != 3 {
		return "", 0, errors.New("invalid address: address did not match")
	}

	localAddress := matches[hostPortRegexAddressIdx]
	localPort, err := strconv.Atoi(matches[hostPortRegexPortIdx])
	if err != nil {
		return "", 0, errors.New("invalid address: port is not a number")
	}
	return localAddress, localPort, nil
}

// FromNetstat parses the output of the netstat command
func FromNetstat(output string) ([]BoundPort, error) {
	lines := strings.Split(output, "\n")

	ports := make([]BoundPort, 0)
	for _, line := range lines {
		// Skip header lines and anything that isn't TCP/UDP.
		if !strings.HasPrefix(line, "tcp") && !strings.HasPrefix(line, "udp") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 6 {
			return nil, fmt.Errorf("unexpected netstat output (too few columns): %s", line)
		}

		transport := parts[0]
		localAddress, localPort, err := parseHostPort(parts[3])
		if err != nil {
			return nil, fmt.Errorf("unexpected netstat output (invalid host address '%s'): %w", parts[3], err)
		}

		// Figure out what column the process detail starts in.
		//
		// For TCP sockets, there's a "State" column which will have a value, but this will be empty for UDP,
		// so we need to go past that column if we're dealing with TCP.
		programIdx := 5
		if strings.HasPrefix(transport, "tcp") {
			programIdx = 6
		}

		// EXAMPLE: 15296/node
		program := parts[programIdx]
		programParts := strings.Split(program, "/")
		pid, err := strconv.Atoi(programParts[0])
		if err != nil {
			return nil, fmt.Errorf("unexpected netstat output (invalid PID): %s", line)
		}

		processName := programParts[1]

		ports = append(ports, newBoundPort(localAddress, localPort, transport, processName, pid))
	}
	return ports, nil
}

// FromSs parses the output of the ss command
func FromSs(output string) ([]BoundPort, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	ports := make([]BoundPort, 0)
	for _, line := range lines {
		// Skip header lines and anything that isn't TCP/UDP.
		if !strings.HasPrefix(line, "tcp") && !strings.HasPrefix(line, "udp") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 7 {
			return nil, fmt.Errorf("unexpected ss output (too few columns): %s", line)
		}

		transport := parts[0]
		localAddress, localPort, err := parseHostPort(parts[4])
		if err != nil {
			return nil, fmt.Errorf("unexpected netstat output (invalid host address '%s'): %w", parts[4], err)
		}

		// EXAMPLE: users:(("node",pid=15296,fd=18))
		programMatches := ssUserRegex.FindAllStringSubmatch(parts[6], -1)
		for _, programMatch := range programMatches {
			if len(programMatch) < 3 {
				return nil, fmt.Errorf("unexpected ss output (invalid program value): %s", line)
			}
			processName := programMatch[ssUserRegexProcessIdx]
			pid, err := strconv.Atoi(programMatch[ssUserRegexPidIdx])
			if err != nil {
				return nil, fmt.Errorf("unexpected ss output (invalid PID): %s", line)
			}

			ports = append(ports, newBoundPort(localAddress, localPort, transport, processName, pid))
		}
	}
	return ports, nil
}
