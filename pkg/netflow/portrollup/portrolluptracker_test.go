// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_portRollupTracker_add_ephemeralDestination(t *testing.T) {
	// Arrange
	tracker := newPortRollupTracker()
	portRollupThreshold := 3

	// Add ports
	tracker.add(80, 2001, portRollupThreshold)
	tracker.add(80, 2002, portRollupThreshold)
	tracker.add(80, 2003, portRollupThreshold)
	tracker.add(80, 2004, portRollupThreshold)

	assert.Equal(t, uint16(3), tracker.getSourcePortCount(80))
	assert.Equal(t, map[uint16]bool{uint16(2001): true, uint16(2002): true, uint16(2003): true}, tracker.sourcePorts[80])

	for _, destPort := range []uint16{2001, 2002, 2003} {
		assert.Equal(t, uint16(1), tracker.getDestPortCount(destPort))
		assert.Equal(t, map[uint16]bool{uint16(80): true}, tracker.destPorts[destPort])
	}

	// make sure no entry is created for port 2004 in `destPorts` since the threshold is already reached
	assert.Equal(t, uint16(0), tracker.getDestPortCount(2004))
	_, exist := tracker.destPorts[2004]
	assert.Equal(t, false, exist)
}

func Test_portRollupTracker_add_ephemeralSource(t *testing.T) {
	// Arrange
	tracker := newPortRollupTracker()
	portRollupThreshold := 3

	// Add ports
	tracker.add(2001, 80, portRollupThreshold)
	tracker.add(2002, 80, portRollupThreshold)
	tracker.add(2003, 80, portRollupThreshold)
	tracker.add(2004, 80, portRollupThreshold)

	assert.Equal(t, uint16(3), tracker.getDestPortCount(80))
	assert.Equal(t, map[uint16]bool{uint16(2001): true, uint16(2002): true, uint16(2003): true}, tracker.destPorts[80])

	for _, sourcePort := range []uint16{2001, 2002, 2003} {
		assert.Equal(t, uint16(1), tracker.getSourcePortCount(sourcePort))
		assert.Equal(t, map[uint16]bool{uint16(80): true}, tracker.sourcePorts[sourcePort])
	}

	// make sure no entry is created for port 2004 in `destPorts` since the threshold is already reached
	assert.Equal(t, uint16(0), tracker.getSourcePortCount(2004))
	_, exist := tracker.sourcePorts[2004]
	assert.Equal(t, false, exist)
}
