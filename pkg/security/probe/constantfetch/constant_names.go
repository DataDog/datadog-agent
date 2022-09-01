// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package constantfetch

const (
	SizeOfInode = "sizeof_inode"
	SizeOfUPID  = "sizeof_upid"
)

// offset names in the structure "OffsetName_[Struct Name]_[Field Name]"
const (
	OffsetName_SuperBlock_SMagic          = "sb_magic_offset"
	OffsetName_SignalStruct_TTY           = "tty_offset"
	OffsetName_TTYStruct_Name             = "tty_name_offset"
	OffsetName_Cred_UID                   = "creds_uid_offset"
	OffsetName_BPFMap_ID                  = "bpf_map_id_offset"
	OffsetName_BPFMap_Name                = "bpf_map_name_offset"
	OffsetName_BPFMap_MapType             = "bpf_map_type_offset"
	OffsetName_BPFProg_Aux                = "bpf_prog_aux_offset"
	OffsetName_BPFProg_Tag                = "bpf_prog_tag_offset"
	OffsetName_BPFProg_Type               = "bpf_prog_type_offset"
	OffsetName_BPFProg_ExpectedAttachType = "bpf_prog_attach_type_offset"
	OffsetName_BPFProgAux_ID              = "bpf_prog_aux_id_offset"
	OffsetName_BPFProgAux_Name            = "bpf_prog_aux_name_offset"
	OffsetName_PID_Level                  = "pid_level_offset"
	OffsetName_PID_Numbers                = "pid_numbers_offset"
	OffsetName_Dentry_DSB                 = "dentry_sb_offset"
	OffsetName_PipeInodeInfo_Bufs         = "pipe_inode_info_bufs_offset"
	OffsetName_NetDevice_IfIndex          = "net_device_ifindex_offset"
	OffsetName_Net_NS                     = "net_ns_offset"
	OffsetName_Net_ProcInum               = "net_proc_inum_offset"
	OffsetName_SockCommon_SKCNet          = "sock_common_skc_net_offset"
	OffsetName_Socket_SK                  = "socket_sock_offset"
	OffsetName_NFConn_CTNet               = "nf_conn_ct_net_offset"
	OffsetName_SockCommon_SKCFamily       = "sock_common_skc_family_offset"
	OffsetName_FlowI4_SADDR               = "flowi4_saddr_offset"
	OffsetName_FlowI6_SADDR               = "flowi6_saddr_offset"
	OffsetName_FlowI4_ULI                 = "flowi4_uli_offset"
	OffsetName_FlowI6_ULI                 = "flowi6_uli_offset"
	OffsetName_LinuxBinprm_File           = "binprm_file_offset"
)
