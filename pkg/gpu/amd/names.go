// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"fmt"
	"regexp"
	"strings"
)

// instinctNames maps PCI device IDs of AMD Instinct accelerators to the
// marketing names of libdrm's amdgpu.ids
// (https://gitlab.freedesktop.org/mesa/drm/-/blob/main/data/amdgpu.ids,
// installed as /usr/share/libdrm/amdgpu.ids), verbatim. It is a fallback for
// ASICs that do not expose the product_name attribute: the MI350X virtual
// function of a DigitalOcean droplet (see testdata/mi350x_vf) has none.
//
// Only IDs that amdgpu.ids maps to a single Instinct name are listed. The file
// is keyed by (device ID, revision ID), but the revision it lists for the
// MI350X VF (C0) differs from the PCI revision in sysfs (0x00), so the device
// ID alone is used. That is ambiguous for the legacy IDs shared between an
// Instinct and a Radeon Pro model (0x66a1, 0x6860, 0x6864, 0x686c), which are
// left out.
//
// The "VF" and "HF" suffixes are part of the amdgpu.ids names and are kept.
var instinctNames = map[uint16]string{
	0x738c: "AMD Instinct MI100",
	0x7408: "AMD Instinct MI250X",
	0x740c: "AMD Instinct MI250X / MI250",
	0x740f: "AMD Instinct MI210",
	0x74a0: "AMD Instinct MI300A",
	0x74a1: "AMD Instinct MI300X",
	0x74a2: "AMD Instinct MI308X",
	0x74a5: "AMD Instinct MI325X",
	0x74a8: "AMD Instinct MI308X HF",
	0x74a9: "AMD Instinct MI300X HF",
	0x74b5: "AMD Instinct MI300X VF",
	0x74b6: "AMD Instinct MI308X",
	0x74bd: "AMD Instinct MI300X HF",
	0x75a0: "AMD Instinct MI350X",
	0x75a3: "AMD Instinct MI355X",
	0x75b0: "AMD Instinct MI350X VF",
	0x75b3: "AMD Instinct MI355X VF",
}

// deviceName returns the product name for a PCI device ID, or a generic name
// carrying the ID when it is not known.
func deviceName(deviceID uint16) string {
	if name, ok := instinctNames[deviceID]; ok {
		return name
	}
	return fmt.Sprintf("AMD GPU 0x%04x", deviceID)
}

// nameSeparatorsRegex matches the characters of a product name that cannot
// appear in the gpu_device tag once the name is lowercased and its spaces are
// replaced by underscores (^[a-z0-9_.-]+$), such as the "/" of
// "AMD Instinct MI250X / MI250".
var nameSeparatorsRegex = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// cleanName makes a product name usable as a tag value: runs of characters
// that are neither alphanumeric nor . _ - are replaced by a single space.
func cleanName(name string) string {
	return strings.TrimSpace(nameSeparatorsRegex.ReplaceAllString(name, " "))
}
