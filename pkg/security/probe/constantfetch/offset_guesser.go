// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf
// +build linux,linux_bpf

package constantfetch

import (
	"math"
	"os"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const offsetGuesserUID = "security-og"

var (
	offsetGuesserMaps = []*manager.Map{
		{Name: "pid_offset"},
	}

	offsetGuesserProbes = []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          offsetGuesserUID,
				EBPFFuncName: "kprobe_get_pid_task",
			},
		},
	}
)

// OffsetGuesser defines an offset guesser object
type OffsetGuesser struct {
	config  *config.Config
	manager *manager.Manager
	res     map[string]uint64
}

// NewOffsetGuesserFetcher returns a new OffsetGuesserFetcher
func NewOffsetGuesserFetcher(config *config.Config) *OffsetGuesser {
	return &OffsetGuesser{
		config: config,
		manager: &manager.Manager{
			Maps:   offsetGuesserMaps,
			Probes: offsetGuesserProbes,
		},
		res: make(map[string]uint64),
	}
}

func (og *OffsetGuesser) String() string {
	return "offset-guesser"
}

func (og *OffsetGuesser) guessPidNumbersOfsset() (uint64, error) {
	if _, err := os.ReadFile(utils.StatusPath(int32(utils.Getpid()))); err != nil {
		return ErrorSentinel, err
	}
	offsetMap, _, err := og.manager.GetMap("pid_offset")
	if err != nil || offsetMap == nil {
		return ErrorSentinel, err
	}

	var key, offset uint32
	if err := offsetMap.Lookup(key, &offset); err != nil {
		return ErrorSentinel, err
	}

	return uint64(offset), nil
}

func (og *OffsetGuesser) guess(id string) error {
	switch id {
	case OffsetNamePIDStructNumbers:
		offset, err := og.guessPidNumbersOfsset()
		if err != nil {
			return err
		}
		og.res[id] = offset
	}

	return nil
}

// AppendSizeofRequest appends a sizeof request
func (og *OffsetGuesser) AppendSizeofRequest(id, typeName, headerName string) {
}

// AppendOffsetofRequest appends an offset request
func (og *OffsetGuesser) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	og.res[id] = ErrorSentinel
}

// FinishAndGetResults returns the results
func (og *OffsetGuesser) FinishAndGetResults() (map[string]uint64, error) {
	loader := ebpf.NewOffsetGuesserLoader(og.config)
	defer loader.Close()

	bytecodeReader, err := loader.Load()
	if err != nil {
		return og.res, err
	}
	defer bytecodeReader.Close()

	options := manager.Options{
		ConstantEditors: []manager.ConstantEditor{
			{
				Name:  "pid_expected",
				Value: uint64(utils.Getpid()),
			},
		},
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
	}

	for _, probe := range probes.AllProbes() {
		options.ExcludedFunctions = append(options.ExcludedFunctions, probe.ProbeIdentificationPair.EBPFFuncName)
	}
	options.ExcludedFunctions = append(options.ExcludedFunctions, probes.GetAllTCProgramFunctions()...)

	if err := og.manager.InitWithOptions(bytecodeReader, options); err != nil {
		return og.res, err
	}

	if err := og.manager.Start(); err != nil {
		return og.res, err
	}

	for id := range og.res {
		if err = og.guess(id); err != nil {
			break
		}
	}

	if err := og.manager.Stop(manager.CleanAll); err != nil {
		return og.res, err
	}

	return og.res, err
}
