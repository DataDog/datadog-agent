// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

// TailCallRouteCallers lists the eBPF programs that can tail call into a given tail call route.
//
// In fentry mode, the kernel requires all the programs sharing a program array to be attached to functions with
// the same prototype, so the program arrays and their tail call targets are cloned for each group of callers sharing a
// prototype. A route that isn't listed here is cloned for every group whose programs use its program array. A listed
// route is only cloned for the groups that contain one of its callers, which keeps the number of programs to load low.
//
// A caller is the program that selects the key of the route before tail calling into the program array, which isn't
// necessarily the program that performs the tail call. For example, a dentry resolver callback key is selected by the
// hook point that starts the resolution, and the tail call to the callback is done by the dentry resolver program.
// A missing caller means that the corresponding events will silently be dropped in fentry mode.
type TailCallRouteCallers struct {
	ProgArrayName string
	Key           uint32
	Callers       []string
}

// syscallExitFuncs returns the names of the fentry programs hooked on the exit of the provided syscalls
func syscallExitFuncs(syscalls ...string) []string {
	if len(RuntimeArch) == 0 {
		resolveRuntimeArch()
	}

	var funcs []string
	for _, syscall := range syscalls {
		for _, section := range expandSyscallSections(syscall, true, Exit) {
			funcs = append(funcs, getFunctionNameFromSection(section))
		}
	}
	return funcs
}

// AllFentryTailCallRouteCallers returns the callers of the tail call routes that are only used by a few hook points
func AllFentryTailCallRouteCallers() []TailCallRouteCallers {
	dentryResolverProgs := "dentry_resolver_kprobe_or_fentry_progs"
	dentryCallbackProgs := "dentry_resolver_kprobe_or_fentry_callbacks"

	mountStageOneCallers := append([]string{
		"hook_mnt_change_mountpoint",
		"hook_make_visible",
		"hook_attach_mnt",
		"hook___attach_mnt",
		"hook_mnt_set_mountpoint",
		"hook_attach_recursive_mnt",
		"hook_propagate_mnt",
	}, syscallExitFuncs("mount")...)

	return []TailCallRouteCallers{
		// dentry resolver programs
		{
			ProgArrayName: dentryResolverProgs,
			Key:           DentryResolverKernInputs,
			Callers: []string{
				"hook_cgroup_procs_write",
				"hook_cgroup1_procs_write",
				"hook_cgroup_tasks_write",
				"hook_cgroup1_tasks_write",
			},
		},
		{
			ProgArrayName: dentryResolverProgs,
			Key:           DentryResolverERPCKey,
			Callers:       []string{"hook_security_file_ioctl"},
		},

		// dentry resolver callbacks
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverSetAttrCallbackKprobeKey,
			Callers:       []string{"hook_security_inode_setattr"},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverMkdirCallbackKprobeKey,
			Callers:       append([]string{"rethook_do_mkdirat", "rethook_filename_mkdirat"}, syscallExitFuncs("mkdir", "mkdirat")...),
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverMountStageOneCallbackKprobeKey,
			Callers:       mountStageOneCallers,
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverMountStageTwoCallbackKprobeKey,
			Callers:       []string{tailCallFnc("dr_mount_stage_one_callback")},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverSecurityInodeRmdirCallbackKprobeKey,
			Callers:       []string{"hook_security_inode_rmdir"},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverSetXAttrCallbackKprobeKey,
			Callers:       []string{"hook_vfs_setxattr", "hook_vfs_removexattr"},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverUnlinkCallbackKprobeKey,
			Callers:       []string{"hook_vfs_unlink"},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverLinkSrcCallbackKprobeKey,
			Callers:       []string{"hook_complete_walk"},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverLinkDstCallbackKprobeKey,
			Callers:       append([]string{"rethook_do_linkat", "rethook_filename_linkat"}, syscallExitFuncs("link", "linkat")...),
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverRenameCallbackKprobeKey,
			Callers:       append([]string{"rethook_do_renameat2", "rethook_filename_renameat2"}, syscallExitFuncs("rename", "renameat", "renameat2")...),
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverSELinuxCallbackKprobeKey,
			Callers: []string{
				"hook_sel_write_disable",
				"hook_sel_write_enforce",
				"hook_sel_write_bool",
				"hook_sel_commit_bools_write",
			},
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverChdirCallbackKprobeKey,
			Callers:       syscallExitFuncs("chdir", "fchdir"),
		},
		{
			ProgArrayName: dentryCallbackProgs,
			Key:           DentryResolverCGroupWriteCallbackKprobeKey,
			Callers: []string{
				"hook_cgroup_procs_write",
				"hook_cgroup1_procs_write",
				"hook_cgroup_tasks_write",
				"hook_cgroup1_tasks_write",
			},
		},

		// network stats flush programs
		{
			ProgArrayName: "flush_network_stats_progs",
			Key:           FlushNetworkStatsExitKey,
			Callers:       []string{"hook_do_exit"},
		},
		{
			ProgArrayName: "flush_network_stats_progs",
			Key:           FlushNetworkStatsExecKey,
			Callers:       []string{"hook_mprotect_fixup"},
		},
	}
}
