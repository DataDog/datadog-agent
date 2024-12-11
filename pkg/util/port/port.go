// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package port provides utilities around host port information.
package port

import (
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
