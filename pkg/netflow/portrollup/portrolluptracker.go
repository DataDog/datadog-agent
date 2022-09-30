// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

type portRollupTracker struct {
	// represent: map[<SOURCE_PORT>]map[<DEST_PORT>]true
	sourcePorts map[uint16]uint8

	// represent: map[<DEST_PORT>]map[<SOURCE_PORT>]true
	destPorts map[uint16]uint8
}

func newPortRollupTracker() *portRollupTracker {
	return &portRollupTracker{
		sourcePorts: make(map[uint16]uint8),
		destPorts:   make(map[uint16]uint8),
	}
}

func (pr *portRollupTracker) getSourcePortCount(port uint16) uint16 {
	// TODO: update to uint8?
	return uint16(pr.sourcePorts[port])
}

func (pr *portRollupTracker) getDestPortCount(port uint16) uint16 {
	return uint16(pr.destPorts[port])
}

func (pr *portRollupTracker) add(sourcePort uint16, destPort uint16, portRollupThreshold int) {
	// we only track additional ports if portRollupThreshold has not been reached
	// if portRollupThreshold is already reached, there is no value to track more ports
	// since we already know we need to rollup

	sourceToDestPorts := int(pr.sourcePorts[sourcePort])
	destToSourcePorts := int(pr.destPorts[destPort])
	if sourceToDestPorts >= portRollupThreshold || destToSourcePorts >= portRollupThreshold {
		return
	}

	pr.sourcePorts[sourcePort]++
	pr.destPorts[destPort]++
}
