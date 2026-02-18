// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

import (
	"fmt"
	"strings"
)

const osDescriptorSep = ":"

// Descriptor provides definition of an OS
type Descriptor struct {
	family       Family
	Flavor       Flavor
	Version      string
	Architecture Architecture
}

func NewDescriptor(f Flavor, version string) Descriptor {
	return NewDescriptorWithArch(f, version, AMD64Arch)
}

func NewDescriptorWithArch(f Flavor, version string, arch Architecture) Descriptor {
	return Descriptor{
		family:       f.Type(),
		Flavor:       f,
		Version:      version,
		Architecture: arch,
	}
}

// String format is <flavor>:<version>(:<arch>)
func DescriptorFromString(descStr string, defaultDescriptor Descriptor) Descriptor {
	if descStr == "" {
		return defaultDescriptor
	}

	parts := strings.Split(descStr, osDescriptorSep)
	if len(parts) < 2 || len(parts) > 3 {
		panic(fmt.Sprintf("invalid OS descriptor string, was: %s", descStr))
	}

	flavor := FlavorFromString(parts[0])
	version := parts[1]

	if len(parts) == 3 {
		return NewDescriptorWithArch(flavor, version, ArchitectureFromString(parts[2]))
	}

	return NewDescriptor(flavor, version)
}

func (d Descriptor) Family() Family {
	return d.family
}

func (d Descriptor) WithArch(a Architecture) Descriptor {
	d.Architecture = a
	return d
}

func (d Descriptor) String() string {
	return strings.Join([]string{d.Flavor.String(), d.Version, string(d.Architecture)}, osDescriptorSep)
}
