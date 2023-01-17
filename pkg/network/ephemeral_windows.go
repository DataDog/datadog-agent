// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
)

type portRangeEndpoint int

const (
	low  portRangeEndpoint = 1
	high portRangeEndpoint = 2
)

var (
	ephemeralRanges = map[ConnectionFamily]map[ConnectionType]map[portRangeEndpoint]uint16{
		AFINET: {
			UDP: {low: 0, high: 0},
			TCP: {low: 0, high: 0},
		},
		AFINET6: {
			UDP: {low: 0, high: 0},
			TCP: {low: 0, high: 0},
		},
	}
	rangeGetOnce = sync.Once{}
	netshRegexp  = regexp.MustCompile(`.*: (\d+)`)
)

func getEphemeralRanges() {
	var families = [...]ConnectionFamily{AFINET, AFINET6}
	var protos = [...]ConnectionType{UDP, TCP}
	for _, f := range families {
		for _, p := range protos {
			l, h, err := getEphemeralRange(f, p)
			if err == nil {
				ephemeralRanges[f][p][low] = l
				ephemeralRanges[f][p][high] = h
			}
		}
	}
}

func getEphemeralRange(f ConnectionFamily, t ConnectionType) (low, hi uint16, err error) {
	var protoarg string
	var familyarg string
	switch f {
	case AFINET6:
		familyarg = "ipv6"
	default:
		familyarg = "ipv4"
	}
	switch t {
	case TCP:
		protoarg = "tcp"
	default:
		protoarg = "udp"
	}
	output, err := exec.Command("netsh", "int", familyarg, "show", "dynamicport", protoarg).Output()
	if err != nil {
		return 0, 0, err
	}
	return parseNetshOutput(string(output))
}

func parseNetshOutput(output string) (low, hi uint16, err error) {
	// output should be of the format
	/*
		Protocol tcp Dynamic Port Range
		---------------------------------
		Start Port      : 49000
		Number of Ports : 16000
	*/
	matches := netshRegexp.FindAllStringSubmatch(output, -1)
	if len(matches) != 2 {
		return 0, 0, fmt.Errorf("could not parse output of netsh")
	}
	portstart, err := strconv.Atoi(matches[0][1])
	if err != nil {
		return 0, 0, err
	}
	plen, err := strconv.Atoi(matches[1][1])
	if err != nil {
		return 0, 0, err
	}
	low = uint16(portstart)
	hi = uint16(portstart + plen - 1)
	return low, hi, nil
}

// IsPortInEphemeralRange returns whether the port is ephemeral based on the OS-specific configuration.
func IsPortInEphemeralRange(f ConnectionFamily, t ConnectionType, p uint16) EphemeralPortType {
	rangeGetOnce.Do(getEphemeralRanges)

	rangeLow := ephemeralRanges[f][t][low]
	rangeHi := ephemeralRanges[f][t][high]
	if rangeLow == 0 || rangeHi == 0 {
		return EphemeralUnknown
	}
	if p >= rangeLow && p <= rangeHi {
		return EphemeralTrue
	}
	return EphemeralFalse
}
