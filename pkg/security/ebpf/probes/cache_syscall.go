// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

func getCacheSyscallTailCallRoutes() []manager.TailCallRoute {
	return []manager.TailCallRoute{
		{
			ProgArrayName: "cache_syscall_progs",
			Key:           CacheSyscallUpdateProcCacheCgroupKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tailCallFnc("update_proc_cache_cgroup"),
			},
		},
	}
}
