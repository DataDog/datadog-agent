// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package uprobe provides functionality for attaching and detaching uprobes to
// processes.
package uprobe

import (
	"errors"
	"fmt"

	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// AttachedProgram represents a program that has been attached to a process.
type AttachedProgram struct {
	processID    process.ID
	loader       *loader.Program
	executable   *link.Executable
	attachpoints []link.Link
}

// Detach detaches the program from the target process.
func (p *AttachedProgram) Detach(_ error) error {
	var retErr error
	for _, attachpoint := range p.attachpoints {
		if err := attachpoint.Close(); err != nil {
			if retErr == nil {
				retErr = err
			} else {
				retErr = errors.Join(retErr, err)
			}
		}
	}
	return retErr
}

// LoaderProgram returns the underlying loader program.
func (p *AttachedProgram) LoaderProgram() *loader.Program {
	return p.loader
}

// ProcessID returns the process ID the program is attached to.
func (p *AttachedProgram) ProcessID() process.ID {
	return p.processID
}

// Attach attaches the provided program to the target process.
func Attach(
	loaded *loader.Program,
	executable process.Executable,
	processID process.ID,
) (*AttachedProgram, error) {
	// A silly thing here is that it's going to call, under the hood,
	// safeelf.Open twice: once for the link package and once for finding the
	// text section offset to translate the attachpoints.
	linkExe, err := link.OpenExecutable(executable.Path)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open executable %s: %w", executable.Path, err,
		)
	}
	elfFile, err := safeelf.Open(executable.Path)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open executable %s: %w", executable.Path, err,
		)
	}
	defer elfFile.Close()

	textSection := elfFile.Section(".text")
	if textSection == nil {
		return nil, errors.New("text section not found")
	}

	// As close to injection as possible, check that executable that we analyzed
	// is the same as the one that we're attaching to.
	currentExe, err := process.ResolveExecutable(kernel.ProcFSRoot(), processID.PID)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to resolve executable for process %s: %w", processID, err,
		)
	}
	if currentExe != executable {
		return nil, fmt.Errorf(
			"executable changed during probe setup: %s != %s", currentExe, executable,
		)
	}

	attached := make([]link.Link, 0, len(loaded.Attachpoints))
	for _, attachpoint := range loaded.Attachpoints {
		addr := attachpoint.PC - textSection.Addr + textSection.Offset
		l, err := linkExe.Uprobe(
			"",
			loaded.BpfProgram,
			&link.UprobeOptions{
				PID:     int(processID.PID),
				Address: addr,
				Offset:  0,
				Cookie:  attachpoint.Cookie,
			},
		)
		if err != nil {
			// Clean up any previously attached probes.
			for _, prev := range attached {
				prev.Close()
			}
			return nil, fmt.Errorf(
				"failed to attach uprobe at 0x%x: %w", addr, err,
			)
		}
		attached = append(attached, l)
	}
	return &AttachedProgram{
		processID:    processID,
		loader:       loaded,
		executable:   linkExe,
		attachpoints: attached,
	}, nil
}
