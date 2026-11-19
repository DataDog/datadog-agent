// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	root := ciliumBPFRoot()
	var st unix.Statfs_t
	if err := unix.Statfs(root, &st); err != nil {
		log.Infof("could not stat %s, so a BPF NAT dataplane can't be ruled out: %s", root, err)
	} else if st.Type != unix.BPF_FS_MAGIC {
		log.Infof("%s is not a bpffs mount, so a BPF NAT dataplane can't be ruled out", root)
	}
	return findBPFNATDataplane(os.DirFS(root))
}

// findBPFNATDataplane returns the first pinned map or directory in bpffs that shows a BPF NAT dataplane is
// running, or "" if there's none.
func findBPFNATDataplane(bpffs fs.FS) string {
	const globals = "tc/globals"
	entries, err := fs.ReadDir(bpffs, globals)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Infof("could not read %s in bpffs, so a BPF NAT dataplane can't be ruled out: %s", globals, err)
	}
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
