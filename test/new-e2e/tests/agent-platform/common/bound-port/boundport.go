// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package boundport provides utilies for getting bound port information
package boundport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
)

// BoundPort represents a port that is bound to a process
type BoundPort interface {
	LocalAddress() string
	LocalPort() int
	Process() string
	PID() int
}

type boundPort struct {
	localAddress string
	localPort    int
	processName  string
	pid          int
}

func (b *boundPort) LocalAddress() string {
	return b.localAddress
}

func (b *boundPort) LocalPort() int {
	return b.localPort
}

func (b *boundPort) Process() string {
	return b.processName
}

func (b *boundPort) PID() int {
	return b.pid
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

// parseHostPort parses a host:port string into a host address and port number
// EXAMPLE: 127.0.0.1:45917
// EXAMPLE: [::]:45917
func parseHostPort(address string) (string, int, error) {
	re := regexp.MustCompile(`(?P<Address>.+):(?P<Port>\d+)`)
	matches := re.FindStringSubmatch(address)
	if len(matches) != 3 {
		return "", 0, fmt.Errorf("address did not match")
	}
	addressIndex := re.SubexpIndex("Address")
	portIndex := re.SubexpIndex("Port")
	hostAddress := matches[addressIndex]
	port, err := strconv.Atoi(matches[portIndex])
	if err != nil {
		return "", 0, fmt.Errorf("port is not a number")
	}

	return hostAddress, port, nil
}

// FromNetstat parses the output of the netstat command
func FromNetstat(output string) ([]BoundPort, error) {
	lines := strings.Split(output, "\n")
	ports := make([]BoundPort, 0)
	for _, line := range lines {
		if !strings.Contains(line, "LISTEN") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 7 {
			return nil, fmt.Errorf("unexpected netstat output: %s", line)
		}

		address, port, err := parseHostPort(parts[3])
		if err != nil {
			return nil, fmt.Errorf("unexpected netstat output: %s", line)
		}

		// EXAMPLE: 15296/node
		program := parts[6]
		programParts := strings.Split(program, "/")
		pid, err := strconv.Atoi(programParts[0])
		if err != nil {
			return nil, fmt.Errorf("unexpected netstat output: %s", line)
		}
		ports = append(ports, &boundPort{
			localAddress: address,
			localPort:    port,
			processName:  programParts[1],
			pid:          pid,
		})
	}
	return ports, nil
}

// FromSs parses the output of the ss command
func FromSs(output string) ([]BoundPort, error) {
	lines := strings.Split(output, "\n")
	ports := make([]BoundPort, 0)
	for _, line := range lines {
		if !strings.Contains(line, "LISTEN") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 6 {
			return nil, fmt.Errorf("unexpected ss output: %s", line)
		}

		address, port, err := parseHostPort(parts[3])
		if err != nil {
			return nil, fmt.Errorf("unexpected ss output: %s", line)
		}

		// EXAMPLE: users:(("node",pid=15296,fd=18))
		program := parts[5]
		re := regexp.MustCompile(`users:\(\("(?P<Process>[^"]+)",pid=(?P<PID>\d+),fd=\d+\)\)`)
		matches := re.FindStringSubmatch(program)
		if len(matches) != 3 {
			return nil, fmt.Errorf("unexpected ss output: %s", line)
		}
		processIndex := re.SubexpIndex("Process")
		pidIndex := re.SubexpIndex("PID")
		process := matches[processIndex]
		pid, err := strconv.Atoi(matches[pidIndex])
		if err != nil {
			return nil, fmt.Errorf("unexpected ss output: %s", line)
		}

		ports = append(ports, &boundPort{
			localAddress: address,
			localPort:    port,
			processName:  process,
			pid:          pid,
		})
	}
	return ports, nil
}
