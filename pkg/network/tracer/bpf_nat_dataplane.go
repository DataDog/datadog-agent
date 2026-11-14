// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"io/fs"
	"os"
	"path"
	"strings"
)

// pinned map name prefixes of BPF dataplanes that do NAT or load balancing in BPF
var bpfNATMapPrefixes = []string{
	// Cilium: connection tracking, SNAT, and service load balancing, IPv4 and IPv6
	"cilium_ct4_", "cilium_ct6_", "cilium_ct_any4_", "cilium_ct_any6_",
	"cilium_snat_v4_", "cilium_snat_v6_",
	"cilium_lb4_", "cilium_lb6_",
	// Calico's eBPF dataplane
	"cali_",
}

// hostBPFNATDataplane looks for a BPF NAT dataplane in the host's bpffs, which is mounted at the same path in the
// system-probe container.
func hostBPFNATDataplane() string {
	return findBPFNATDataplane(os.DirFS(ciliumBPFRoot()))
}

// findBPFNATDataplane returns the first pinned map or directory in bpffs that shows a BPF NAT dataplane is
// running, or "" if there's none.
func findBPFNATDataplane(bpffs fs.FS) string {
	const globals = "tc/globals"
	entries, _ := fs.ReadDir(bpffs, globals)
	for _, e := range entries {
		for _, prefix := range bpfNATMapPrefixes {
			if strings.HasPrefix(e.Name(), prefix) {
				return path.Join(globals, e.Name())
			}
		}
	}
	// older Calico releases pin their maps here
	if fi, err := fs.Stat(bpffs, "calico"); err == nil && fi.IsDir() {
		return "calico"
	}
	return ""
}
