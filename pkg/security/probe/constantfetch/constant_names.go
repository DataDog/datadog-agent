// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package constantfetch

// Struct sizes
const (
	SizeOfInode = "sizeof_inode"
	SizeOfUPID  = "sizeof_upid"
)

// Offset names in the format "OffsetName[Struct Name]Struct[Field Name]"
const (
	OffsetNameSuperBlockStructSMagic = "sb_magic_offset"
	OffsetNameSignalStructStructTTY  = "tty_offset"
	OffsetNameTTYStructStructName    = "tty_name_offset"
	OffsetNameCredStructUID          = "creds_uid_offset"
	OffsetNameLinuxBinprmP           = "linux_binprm_p_offset"
	OffsetNameLinuxBinprmArgc        = "linux_binprm_argc_offset"
	OffsetNameLinuxBinprmEnvc        = "linux_binprm_envc_offset"

	// bpf offsets
	OffsetNameBPFMapStructID                  = "bpf_map_id_offset"
	OffsetNameBPFMapStructName                = "bpf_map_name_offset"
	OffsetNameBPFMapStructMapType             = "bpf_map_type_offset"
	OffsetNameBPFProgStructAux                = "bpf_prog_aux_offset"
	OffsetNameBPFProgStructTag                = "bpf_prog_tag_offset"
	OffsetNameBPFProgStructType               = "bpf_prog_type_offset"
	OffsetNameBPFProgStructExpectedAttachType = "bpf_prog_attach_type_offset"
	OffsetNameBPFProgAuxStructID              = "bpf_prog_aux_id_offset"
	OffsetNameBPFProgAuxStructName            = "bpf_prog_aux_name_offset"

	// namespace nr offsets
	OffsetNamePIDStructLevel    = "pid_level_offset"
	OffsetNamePIDStructNumbers  = "pid_numbers_offset"
	OffsetNameDentryStructDSB   = "dentry_sb_offset"
	OffsetNameTaskStructPID     = "task_struct_pid_offset"      // kernels >= 4.19
	OffsetNameTaskStructPIDLink = "task_struct_pid_link_offset" // kernels < 4.19
	OffsetNamePIDLinkStructPID  = "pid_link_pid_offset"         // kernels < 4.19

	// splice event
	OffsetNamePipeInodeInfoStructBufs     = "pipe_inode_info_bufs_offset"
	OffsetNamePipeInodeInfoStructNrbufs   = "pipe_inode_info_nrbufs_offset"    // kernels < 5.5
	OffsetNamePipeInodeInfoStructCurbuf   = "pipe_inode_info_curbuf_offset"    // kernels < 5.5
	OffsetNamePipeInodeInfoStructBuffers  = "pipe_inode_info_buffers_offset"   // kernels < 5.5
	OffsetNamePipeInodeInfoStructHead     = "pipe_inode_info_head_offset"      // kernels >= 5.5
	OffsetNamePipeInodeInfoStructRingsize = "pipe_inode_info_ring_size_offset" // kernels >= 5.5

	// network related constants
	OffsetNameNetDeviceStructIfIndex    = "net_device_ifindex_offset"
	OffsetNameNetStructNS               = "net_ns_offset"
	OffsetNameNetStructProcInum         = "net_proc_inum_offset"
	OffsetNameSockCommonStructSKCNet    = "sock_common_skc_net_offset"
	OffsetNameSocketStructSK            = "socket_sock_offset"
	OffsetNameNFConnStructCTNet         = "nf_conn_ct_net_offset"
	OffsetNameSockCommonStructSKCFamily = "sock_common_skc_family_offset"
	OffsetNameFlowI4StructSADDR         = "flowi4_saddr_offset"
	OffsetNameFlowI6StructSADDR         = "flowi6_saddr_offset"
	OffsetNameFlowI4StructULI           = "flowi4_uli_offset"
	OffsetNameFlowI6StructULI           = "flowi6_uli_offset"

	// Interpreter constants
	OffsetNameLinuxBinprmStructFile = "binprm_file_offset"

	// iouring constants
	OffsetNameIoKiocbStructCtx = "iokiocb_ctx_offset"
)
