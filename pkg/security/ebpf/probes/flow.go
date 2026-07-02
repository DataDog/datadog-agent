// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

// TaskFileIterResolveFlowPidFunc is the name of the iter/task_file eBPF program
// that snapshots the flow_pid map from the open sockets of existing processes.
// It is attached and run manually during the snapshot phase rather than being
// activated like a regular probe.
const TaskFileIterResolveFlowPidFunc = "bpf_iter__task_file_resolve_flow_pid"

// ProcfsFlowPidSnapshotFuncs are the kprobes that populate the flow_pid map from
// procfs when the agent walks /proc/<pid>/fd during the snapshot. They are the
// fallback for kernels without iter/task_file support (< 5.11) and are activated
// only when TaskFileIterResolveFlowPidFunc isn't used.
var ProcfsFlowPidSnapshotFuncs = []string{
	"hook_path_get",
	"hook_proc_fd_link",
}

func getFlowProbes() []*manager.Probe {
	flowProbes := []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: TaskFileIterResolveFlowPidFunc,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_security_sk_classify_flow",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_inet_release",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_inet_csk_destroy_sock",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_sk_destruct",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_inet_put_port",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_sk_common_release",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_inet_shutdown",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_inet_bind",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "rethook_inet_bind",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_inet6_bind",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "rethook_inet6_bind",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_nf_nat_manip_pkt",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_nf_nat_packet",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_nf_ct_delete",
			},
		},
	}

	// procfs snapshot fallback used on kernels without iter/task_file support
	// (< 5.11); activated only when the task_file iterator isn't used.
	for _, fnc := range ProcfsFlowPidSnapshotFuncs {
		flowProbes = append(flowProbes, &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: fnc,
			},
		})
	}

	return flowProbes
}

// GetAllFlushNetworkStatsTaillCallFunctions returns the list of network flush tail call functions
func GetAllFlushNetworkStatsTaillCallFunctions() []string {
	return []string{
		tailCallFnc("flush_network_stats_exec"),
		tailCallFnc("flush_network_stats_exit"),
	}
}

func getFlushNetworkStatsTailCallRoutes() []manager.TailCallRoute {
	return []manager.TailCallRoute{
		{
			ProgArrayName: "flush_network_stats_progs",
			Key:           FlushNetworkStatsExecKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tailCallFnc("flush_network_stats_exec"),
			},
		},
		{
			ProgArrayName: "flush_network_stats_progs",
			Key:           FlushNetworkStatsExitKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tailCallFnc("flush_network_stats_exit"),
			},
		},
	}
}
