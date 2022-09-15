// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

type portRollupTracker struct {
	// represent: map[<SOURCE_PORT>]map[<DEST_PORT>]true
	sourcePorts map[uint16]map[uint16]bool

	// represent: map[<DEST_PORT>]map[<SOURCE_PORT>]true
	destPorts map[uint16]map[uint16]bool
}

func newPortRollupTracker() *portRollupTracker {
	return &portRollupTracker{
		sourcePorts: make(map[uint16]map[uint16]bool),
		destPorts:   make(map[uint16]map[uint16]bool),
	}
}

func (pr *portRollupTracker) getSourcePortCount(port uint16) uint16 {
	return uint16(len(pr.sourcePorts[port]))
}

func (pr *portRollupTracker) getDestPortCount(port uint16) uint16 {
	return uint16(len(pr.destPorts[port]))
}

func (pr *portRollupTracker) add(sourcePort uint16, destPort uint16, portRollupThreshold int) {
	// we only track additional ports if portRollupThreshold has not been reached
	// if portRollupThreshold is already reached, there is no value to track more ports
	// since we already know we need to rollup

	sourceToDestPorts := len(pr.sourcePorts[sourcePort])
	destToSourcePorts := len(pr.destPorts[destPort])
	if sourceToDestPorts >= portRollupThreshold || destToSourcePorts >= portRollupThreshold {
		return
	}

	addPort(pr.sourcePorts, sourcePort, destPort, portRollupThreshold)
	addPort(pr.destPorts, destPort, sourcePort, portRollupThreshold)
}

func addPort(ports map[uint16]map[uint16]bool, port uint16, ephemeralPort uint16, portRollupThreshold int) {
	if _, ok := ports[port]; !ok {
		ports[port] = make(map[uint16]bool)
	}

	if !ports[port][ephemeralPort] {
		ports[port][ephemeralPort] = true
	}
}
