// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package object abstracts the loading of debugging sections from an object
// file (such as an ELF file).
package object

import (
	"debug/dwarf"
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
)

// Architecture is the architecture of the object file.
type Architecture = bininspect.GoArch

// File is an interface that represents an object file.
type File interface {
	io.Closer

	// Access to the DWARF sections.
	DwarfSections() *DebugSections
	// Access to the DWARF data.
	DwarfData() *dwarf.Data
	// LoclistReader returns a reader that can be used to read
	// loclist entries. The reader is not safe for concurrent use.
	LoclistReader() (*loclist.Reader, error)
	// PointerSize returns the size of a pointer on the architecture of the object file.
	PointerSize() uint8
}
