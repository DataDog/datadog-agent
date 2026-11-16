// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package pci reads static PCI device facts (NUMA locality, upstream bridges)
// from sysfs, and normalizes PCI bus IDs across their NVML and sysfs spellings.
package pci

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// NormalizeBusID returns a PCI bus ID as sysfs spells it: a lowercase
// domain:bus:device.function with the domain padded to at least four hex
// digits. NVML pads the domain to eight digits ("00000000:8D:00.0"), sysfs to
// four ("0000:8d:00.0"). Domains above 0xffff keep all their digits.
func NormalizeBusID(busID string) (string, error) {
	domain, rest, ok := strings.Cut(strings.ToLower(strings.TrimSpace(busID)), ":")
	if !ok {
		return "", fmt.Errorf("invalid PCI bus ID %q", busID)
	}
	bus, rest, ok := strings.Cut(rest, ":")
	if !ok {
		return "", fmt.Errorf("invalid PCI bus ID %q", busID)
	}
	device, function, ok := strings.Cut(rest, ".")
	if !ok {
		return "", fmt.Errorf("invalid PCI bus ID %q", busID)
	}
	parts := [4]struct {
		text    string
		maxLen  int
		bitSize int
	}{{domain, 8, 32}, {bus, 2, 8}, {device, 2, 5}, {function, 1, 3}}
	var values [4]uint64
	for i, part := range parts {
		if part.text == "" || len(part.text) > part.maxLen {
			return "", fmt.Errorf("invalid PCI bus ID %q", busID)
		}
		value, err := strconv.ParseUint(part.text, 16, part.bitSize)
		if err != nil {
			return "", fmt.Errorf("invalid PCI bus ID %q: %w", busID, err)
		}
		values[i] = value
	}
	return fmt.Sprintf("%04x:%02x:%02x.%x", values[0], values[1], values[2], values[3]), nil
}

// Sysfs reads PCI device attributes from sysfs mounts.
type Sysfs struct {
	devicesDirs []string
}

// NewSysfs reads from the given sysfs mount points (e.g. "/host/sys", "/sys"),
// in order of preference. Containers often mount the host sysfs only partly,
// and the PCI device tree isn't namespaced, so the container's own /sys is a
// useful fallback after the host one.
func NewSysfs(sysRoots ...string) *Sysfs {
	s := &Sysfs{}
	seen := make(map[string]bool, len(sysRoots))
	for _, root := range sysRoots {
		dir := filepath.Join(root, "bus", "pci", "devices")
		if root != "" && !seen[dir] {
			seen[dir] = true
			s.devicesDirs = append(s.devicesDirs, dir)
		}
	}
	return s
}

// NUMANode returns the host NUMA node of a PCI device. It reports false when
// the device is not found, the attribute is missing, or the kernel reports -1
// (unknown). Node 0 is valid, and a node may have no CPUs.
func (s *Sysfs) NUMANode(busID string) (int, bool) {
	address, err := NormalizeBusID(busID)
	if err != nil {
		return -1, false
	}
	for _, dir := range s.devicesDirs {
		content, err := os.ReadFile(filepath.Join(dir, address, "numa_node"))
		if err != nil {
			continue
		}
		node, err := strconv.Atoi(strings.TrimSpace(string(content)))
		if err != nil || node < 0 {
			return -1, false
		}
		return node, true
	}
	return -1, false
}

// UpstreamBridges returns the normalized bus IDs of the bridges between a PCI
// device and its root port, ordered from the device toward the root port and
// excluding the device itself. A device directly on a root complex has no
// bridges.
//
// The path is read from the target of the /sys/bus/pci/devices/<bdf> symlink
// (e.g. ../../../devices/pci0000:89/0000:89:00.0/0000:8a:00.0/0000:8d:00.0),
// so it does not need the /sys/devices tree to be mounted.
func (s *Sysfs) UpstreamBridges(busID string) ([]string, error) {
	address, err := NormalizeBusID(busID)
	if err != nil {
		return nil, err
	}
	lastErr := errors.New("no sysfs PCI devices directory")
	for _, dir := range s.devicesDirs {
		target, err := os.Readlink(filepath.Join(dir, address))
		if err != nil {
			lastErr = err
			continue
		}
		bridges, err := bridgesFromDevicePath(target, address)
		if err != nil {
			lastErr = err
			continue
		}
		return bridges, nil
	}
	return nil, lastErr
}

func bridgesFromDevicePath(target, address string) ([]string, error) {
	components := strings.Split(filepath.ToSlash(filepath.Clean(target)), "/")
	last := len(components) - 1
	if device, err := NormalizeBusID(components[last]); err != nil || device != address {
		return nil, fmt.Errorf("sysfs path %q does not end at PCI device %s", target, address)
	}
	bridges := []string{}
	for i := last - 1; i >= 0; i-- {
		component := components[i]
		// The path reaches the root complex at its host bridge, e.g.
		// pci0000:89 or a VMD domain such as pci10000:e0.
		if strings.HasPrefix(component, "pci") {
			return bridges, nil
		}
		bridge, err := NormalizeBusID(component)
		if err != nil {
			return nil, fmt.Errorf("unexpected component %q in sysfs path %q", component, target)
		}
		bridges = append(bridges, bridge)
	}
	return nil, fmt.Errorf("sysfs path %q does not reach a PCI host bridge", target)
}
