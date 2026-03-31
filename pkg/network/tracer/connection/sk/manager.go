// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package sk

import (
	"path/filepath"
	"strings"
	"syscall"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

func initManager(mgr *ddebpf.Manager) {
	mgr.Maps = []*manager.Map{
		{Name: "sk_tcp_stats"},
		{Name: "sk_udp_stats"},
		{Name: "port_bindings"},
		{Name: "udp_port_bindings"},
	}
	for funcName := range programs {
		p := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: funcName,
				UID:          probeUID,
			},
		}
		if strings.HasSuffix(funcName, "_sockops") || strings.HasSuffix(funcName, "_cgroup") {
			cpath, err := findCgroupPath()
			if err != nil {
				continue
			}
			p.CGroupPath = cpath
		}
		mgr.Probes = append(mgr.Probes, p)
	}
}

var findCgroupPath = funcs.Memoize(func() (string, error) {
	cgroupPath := "/sys/fs/cgroup"

	var st syscall.Statfs_t
	err := syscall.Statfs(cgroupPath, &st)
	if err != nil {
		return "", err
	}
	isCgroupV2Enabled := st.Type == unix.CGROUP2_SUPER_MAGIC
	if !isCgroupV2Enabled {
		cgroupPath = filepath.Join(cgroupPath, "unified")
	}
	return cgroupPath, nil
})
