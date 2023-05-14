// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package offsetguess

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	manager "github.com/DataDog/ebpf-manager"
)

var zero uint64

// These constants should be in sync with the equivalent definitions in the ebpf program.
const (
	disabled uint8 = 0
	enabled  uint8 = 1
)

const (
	// The source port is much further away in the inet sock.
	thresholdInetSock = 2000

	notApplicable = 99999 // An arbitrary large number to indicate that the value should be ignored
)

var stateString = map[State]string{
	StateUninitialized: "uninitialized",
	StateChecking:      "checking",
	StateChecked:       "checked",
	StateReady:         "ready",
}

var whatString = map[GuessWhat]string{
	GuessSAddr:     "source address",
	GuessDAddr:     "destination address",
	GuessFamily:    "family",
	GuessSPort:     "source port",
	GuessDPort:     "destination port",
	GuessNetNS:     "network namespace",
	GuessRTT:       "Round Trip Time",
	GuessDAddrIPv6: "destination address IPv6",

	// Guess offsets in struct flowi4
	GuessSAddrFl4: "source address flowi4",
	GuessDAddrFl4: "destination address flowi4",
	GuessSPortFl4: "source port flowi4",
	GuessDPortFl4: "destination port flowi4",

	// Guess offsets in struct flowi6
	GuessSAddrFl6: "source address flowi6",
	GuessDAddrFl6: "destination address flowi6",
	GuessSPortFl6: "source port flowi6",
	GuessDPortFl6: "destination port flowi6",

	GuessSocketSK:              "sk field on struct socket",
	GuessSKBuffSock:            "sk field on struct sk_buff",
	GuessSKBuffTransportHeader: "transport header field on struct sk_buff",
	GuessSKBuffHead:            "head field on struct sk_buff",

	GuessCtTupleOrigin: "conntrack origin tuple",
	GuessCtTupleReply:  "conntrack reply tuple",
	GuessCtStatus:      "conntrack status",
	GuessCtNet:         "conntrack network namespace",
}

type OffsetGuesser interface {
	Manager() *manager.Manager
	Probes(c *config.Config) (map[string]struct{}, error)
	Guess(c *config.Config) ([]manager.ConstantEditor, error)
	Close()
}

type fieldValues struct {
	saddr     uint32
	daddr     uint32
	sport     uint16
	dport     uint16
	netns     uint32
	family    uint16
	rtt       uint32
	rttVar    uint32
	daddrIPv6 [4]uint32

	// Used for guessing offsets in struct flowi4
	saddrFl4 uint32
	daddrFl4 uint32
	sportFl4 uint16
	dportFl4 uint16

	// Used for guessing offsets in struct flowi6
	saddrFl6 [4]uint32
	daddrFl6 [4]uint32
	sportFl6 uint16
	dportFl6 uint16

	ctStatus uint32
}

func idPair(name probes.ProbeFuncName) manager.ProbeIdentificationPair {
	return manager.ProbeIdentificationPair{
		EBPFFuncName: name,
		UID:          "offset",
	}
}

func enableProbe(enabled map[probes.ProbeFuncName]struct{}, name probes.ProbeFuncName) {
	enabled[name] = struct{}{}
}

func SetupOffsetGuesser(guesser OffsetGuesser, config *config.Config, buf bytecode.AssetReader) error {
	// Enable kernel probes used for offset guessing.
	offsetMgr := guesser.Manager()
	offsetOptions := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
	}
	enabledProbes, err := guesser.Probes(config)
	if err != nil {
		return fmt.Errorf("unable to configure offset guessing probes: %w", err)
	}

	for _, p := range offsetMgr.Probes {
		if _, enabled := enabledProbes[p.EBPFFuncName]; !enabled {
			offsetOptions.ExcludedFunctions = append(offsetOptions.ExcludedFunctions, p.EBPFFuncName)
		}
	}
	for funcName := range enabledProbes {
		offsetOptions.ActivatedProbes = append(
			offsetOptions.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: funcName,
					UID:          "offset",
				},
			})
	}
	if err := offsetMgr.InitWithOptions(buf, offsetOptions); err != nil {
		return fmt.Errorf("could not load bpf module for offset guessing: %s", err)
	}

	if err := offsetMgr.Start(); err != nil {
		return fmt.Errorf("could not start offset ebpf manager: %s", err)
	}

	return nil
}
