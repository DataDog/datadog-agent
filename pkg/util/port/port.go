// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package port provides utilities around host port information.
package port

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/port/portlist"
)

// Port is a used port on the machine
type Port = portlist.Port

// GetUsedPorts returns the list of used ports
func GetUsedPorts() ([]Port, error) {
	poller := portlist.Poller{
		IncludeLocalhost: true,
	}
	ports, _, err := poller.Poll()
	if err != nil {
		return nil, err
	}

	err = poller.Close()
	return ports, err
}

// GetProcessFromPort returns the process name and pid using a given port
func GetProcessFromPort(port int, protocol string) (string, int, error) {
	ports, err := GetUsedPorts()
	if err != nil {
		return "", 0, err
	}

	for _, p := range ports {
		if p.Port == uint16(port) && p.Proto == protocol {
			return p.Process, p.Pid, nil
		}
	}

	return "", 0, fmt.Errorf("port %d is not used", port)
}

// GetUsedPortsForPid returns a list of ports used by a process
func GetUsedPortsForPid(pid int) ([]Port, error) {
	ports, err := GetUsedPorts()
	if err != nil {
		return nil, err
	}

	var ret []Port
	for _, p := range ports {
		if p.Pid == pid {
			ret = append(ret, p)
		}
	}
	return ret, nil
}
