// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

	lru "github.com/hashicorp/golang-lru"
)

const (
	// MaxSegmentLength defines the maximum length of each segment of a path
	MaxSegmentLength = 255

	// MaxPathDepth defines the maximum depth of a path
	// see pkg/security/ebpf/c/dentry_resolver.h: DR_MAX_TAIL_CALL * DR_MAX_ITERATION_DEPTH
	MaxPathDepth = 1350

	// MaxBpfObjName defines the maximum length of a Bpf object name
	MaxBpfObjName = 16

	// PathSuffix defines the suffix used for path fields
	PathSuffix = ".path"

	// NameSuffix defines the suffix used for name fields
	NameSuffix = ".name"

	// ContainerIDLen defines the length of a container ID
	ContainerIDLen = sha256.Size * 2
)

var (
	// vmConstants is the list of protection flags for a virtual memory segment
	// generate_constants:Virtual Memory flags,Virtual Memory flags define the protection of a virtual memory segment.
	vmConstants = map[string]int{
		"VM_NONE":         0x0,
		"VM_READ":         0x1,
		"VM_WRITE":        0x2,
		"VM_EXEC":         0x4,
		"VM_SHARED":       0x8,
		"VM_MAYREAD":      0x00000010,
		"VM_MAYWRITE":     0x00000020,
		"VM_MAYEXEC":      0x00000040,
		"VM_MAYSHARE":     0x00000080,
		"VM_GROWSDOWN":    0x00000100, /* general info on the segment */
		"VM_UFFD_MISSING": 0x00000200, /* missing pages tracking */
		"VM_PFNMAP":       0x00000400, /* Page-ranges managed without "struct page", just pure PFN */
		"VM_UFFD_WP":      0x00001000, /* wrprotect pages tracking */
		"VM_LOCKED":       0x00002000,
		"VM_IO":           0x00004000, /* Memory mapped I/O or similar */
		"VM_SEQ_READ":     0x00008000, /* App will access data sequentially */
		"VM_RAND_READ":    0x00010000, /* App will not benefit from clustered reads */
		"VM_DONTCOPY":     0x00020000, /* Do not copy this vma on fork */
		"VM_DONTEXPAND":   0x00040000, /* Cannot expand with mremap() */
		"VM_LOCKONFAULT":  0x00080000, /* Lock the pages covered when they are faulted in */
		"VM_ACCOUNT":      0x00100000, /* Is a VM accounted object */
		"VM_NORESERVE":    0x00200000, /* should the VM suppress accounting */
		"VM_HUGETLB":      0x00400000, /* Huge TLB Page VM */
		"VM_SYNC":         0x00800000, /* Synchronous page faults */
		"VM_ARCH_1":       0x01000000, /* Architecture-specific flag */
		"VM_WIPEONFORK":   0x02000000, /* Wipe VMA contents in child. */
		"VM_DONTDUMP":     0x04000000, /* Do not include in the core dump */
		"VM_SOFTDIRTY":    0x08000000, /* Not soft dirty clean area */
		"VM_MIXEDMAP":     0x10000000, /* Can contain "struct page" and pure PFN pages */
		"VM_HUGEPAGE":     0x20000000, /* MADV_HUGEPAGE marked this vma */
		"VM_NOHUGEPAGE":   0x40000000, /* MADV_NOHUGEPAGE marked this vma */
		"VM_MERGEABLE":    0x80000000, /* KSM may merge identical pages */
	}

	// BPFCmdConstants is the list of BPF commands
	// generate_constants:BPF commands,BPF commands are used to specify a command to a bpf syscall.
	BPFCmdConstants = map[string]BPFCmd{
		"BPF_MAP_CREATE":                  BpfMapCreateCmd,
		"BPF_MAP_LOOKUP_ELEM":             BpfMapLookupElemCmd,
		"BPF_MAP_UPDATE_ELEM":             BpfMapUpdateElemCmd,
		"BPF_MAP_DELETE_ELEM":             BpfMapDeleteElemCmd,
		"BPF_MAP_GET_NEXT_KEY":            BpfMapGetNextKeyCmd,
		"BPF_PROG_LOAD":                   BpfProgLoadCmd,
		"BPF_OBJ_PIN":                     BpfObjPinCmd,
		"BPF_OBJ_GET":                     BpfObjGetCmd,
		"BPF_PROG_ATTACH":                 BpfProgAttachCmd,
		"BPF_PROG_DETACH":                 BpfProgDetachCmd,
		"BPF_PROG_TEST_RUN":               BpfProgTestRunCmd,
		"BPF_PROG_RUN":                    BpfProgTestRunCmd,
		"BPF_PROG_GET_NEXT_ID":            BpfProgGetNextIDCmd,
		"BPF_MAP_GET_NEXT_ID":             BpfMapGetNextIDCmd,
		"BPF_PROG_GET_FD_BY_ID":           BpfProgGetFdByIDCmd,
		"BPF_MAP_GET_FD_BY_ID":            BpfMapGetFdByIDCmd,
		"BPF_OBJ_GET_INFO_BY_FD":          BpfObjGetInfoByFdCmd,
		"BPF_PROG_QUERY":                  BpfProgQueryCmd,
		"BPF_RAW_TRACEPOINT_OPEN":         BpfRawTracepointOpenCmd,
		"BPF_BTF_LOAD":                    BpfBtfLoadCmd,
		"BPF_BTF_GET_FD_BY_ID":            BpfBtfGetFdByIDCmd,
		"BPF_TASK_FD_QUERY":               BpfTaskFdQueryCmd,
		"BPF_MAP_LOOKUP_AND_DELETE_ELEM":  BpfMapLookupAndDeleteElemCmd,
		"BPF_MAP_FREEZE":                  BpfMapFreezeCmd,
		"BPF_BTF_GET_NEXT_ID":             BpfBtfGetNextIDCmd,
		"BPF_MAP_LOOKUP_BATCH":            BpfMapLookupBatchCmd,
		"BPF_MAP_LOOKUP_AND_DELETE_BATCH": BpfMapLookupAndDeleteBatchCmd,
		"BPF_MAP_UPDATE_BATCH":            BpfMapUpdateBatchCmd,
		"BPF_MAP_DELETE_BATCH":            BpfMapDeleteBatchCmd,
		"BPF_LINK_CREATE":                 BpfLinkCreateCmd,
		"BPF_LINK_UPDATE":                 BpfLinkUpdateCmd,
		"BPF_LINK_GET_FD_BY_ID":           BpfLinkGetFdByIDCmd,
		"BPF_LINK_GET_NEXT_ID":            BpfLinkGetNextIDCmd,
		"BPF_ENABLE_STATS":                BpfEnableStatsCmd,
		"BPF_ITER_CREATE":                 BpfIterCreateCmd,
		"BPF_LINK_DETACH":                 BpfLinkDetachCmd,
		"BPF_PROG_BIND_MAP":               BpfProgBindMapCmd,
	}

	// BPFHelperFuncConstants is the list of BPF helper func constants
	// generate_constants:BPF helper functions,BPF helper functions are the supported BPF helper functions.
	BPFHelperFuncConstants = map[string]BPFHelperFunc{
		"BPF_UNSPEC":                         BpfUnspec,
		"BPF_MAP_LOOKUP_ELEM":                BpfMapLookupElem,
		"BPF_MAP_UPDATE_ELEM":                BpfMapUpdateElem,
		"BPF_MAP_DELETE_ELEM":                BpfMapDeleteElem,
		"BPF_PROBE_READ":                     BpfProbeRead,
		"BPF_KTIME_GET_NS":                   BpfKtimeGetNs,
		"BPF_TRACE_PRINTK":                   BpfTracePrintk,
		"BPF_GET_PRANDOM_U32":                BpfGetPrandomU32,
		"BPF_GET_SMP_PROCESSOR_ID":           BpfGetSmpProcessorID,
		"BPF_SKB_STORE_BYTES":                BpfSkbStoreBytes,
		"BPF_L3_CSUM_REPLACE":                BpfL3CsumReplace,
		"BPF_L4_CSUM_REPLACE":                BpfL4CsumReplace,
		"BPF_TAIL_CALL":                      BpfTailCall,
		"BPF_CLONE_REDIRECT":                 BpfCloneRedirect,
		"BPF_GET_CURRENT_PID_TGID":           BpfGetCurrentPidTgid,
		"BPF_GET_CURRENT_UID_GID":            BpfGetCurrentUIDGid,
		"BPF_GET_CURRENT_COMM":               BpfGetCurrentComm,
		"BPF_GET_CGROUP_CLASSID":             BpfGetCgroupClassid,
		"BPF_SKB_VLAN_PUSH":                  BpfSkbVlanPush,
		"BPF_SKB_VLAN_POP":                   BpfSkbVlanPop,
		"BPF_SKB_GET_TUNNEL_KEY":             BpfSkbGetTunnelKey,
		"BPF_SKB_SET_TUNNEL_KEY":             BpfSkbSetTunnelKey,
		"BPF_PERF_EVENT_READ":                BpfPerfEventRead,
		"BPF_REDIRECT":                       BpfRedirect,
		"BPF_GET_ROUTE_REALM":                BpfGetRouteRealm,
		"BPF_PERF_EVENT_OUTPUT":              BpfPerfEventOutput,
		"BPF_SKB_LOAD_BYTES":                 BpfSkbLoadBytes,
		"BPF_GET_STACKID":                    BpfGetStackid,
		"BPF_CSUM_DIFF":                      BpfCsumDiff,
		"BPF_SKB_GET_TUNNEL_OPT":             BpfSkbGetTunnelOpt,
		"BPF_SKB_SET_TUNNEL_OPT":             BpfSkbSetTunnelOpt,
		"BPF_SKB_CHANGE_PROTO":               BpfSkbChangeProto,
		"BPF_SKB_CHANGE_TYPE":                BpfSkbChangeType,
		"BPF_SKB_UNDER_CGROUP":               BpfSkbUnderCgroup,
		"BPF_GET_HASH_RECALC":                BpfGetHashRecalc,
		"BPF_GET_CURRENT_TASK":               BpfGetCurrentTask,
		"BPF_PROBE_WRITE_USER":               BpfProbeWriteUser,
		"BPF_CURRENT_TASK_UNDER_CGROUP":      BpfCurrentTaskUnderCgroup,
		"BPF_SKB_CHANGE_TAIL":                BpfSkbChangeTail,
		"BPF_SKB_PULL_DATA":                  BpfSkbPullData,
		"BPF_CSUM_UPDATE":                    BpfCsumUpdate,
		"BPF_SET_HASH_INVALID":               BpfSetHashInvalid,
		"BPF_GET_NUMA_NODE_ID":               BpfGetNumaNodeID,
		"BPF_SKB_CHANGE_HEAD":                BpfSkbChangeHead,
		"BPF_XDP_ADJUST_HEAD":                BpfXdpAdjustHead,
		"BPF_PROBE_READ_STR":                 BpfProbeReadStr,
		"BPF_GET_SOCKET_COOKIE":              BpfGetSocketCookie,
		"BPF_GET_SOCKET_UID":                 BpfGetSocketUID,
		"BPF_SET_HASH":                       BpfSetHash,
		"BPF_SETSOCKOPT":                     BpfSetsockopt,
		"BPF_SKB_ADJUST_ROOM":                BpfSkbAdjustRoom,
		"BPF_REDIRECT_MAP":                   BpfRedirectMap,
		"BPF_SK_REDIRECT_MAP":                BpfSkRedirectMap,
		"BPF_SOCK_MAP_UPDATE":                BpfSockMapUpdate,
		"BPF_XDP_ADJUST_META":                BpfXdpAdjustMeta,
		"BPF_PERF_EVENT_READ_VALUE":          BpfPerfEventReadValue,
		"BPF_PERF_PROG_READ_VALUE":           BpfPerfProgReadValue,
		"BPF_GETSOCKOPT":                     BpfGetsockopt,
		"BPF_OVERRIDE_RETURN":                BpfOverrideReturn,
		"BPF_SOCK_OPS_CB_FLAGS_SET":          BpfSockOpsCbFlagsSet,
		"BPF_MSG_REDIRECT_MAP":               BpfMsgRedirectMap,
		"BPF_MSG_APPLY_BYTES":                BpfMsgApplyBytes,
		"BPF_MSG_CORK_BYTES":                 BpfMsgCorkBytes,
		"BPF_MSG_PULL_DATA":                  BpfMsgPullData,
		"BPF_BIND":                           BpfBind,
		"BPF_XDP_ADJUST_TAIL":                BpfXdpAdjustTail,
		"BPF_SKB_GET_XFRM_STATE":             BpfSkbGetXfrmState,
		"BPF_GET_STACK":                      BpfGetStack,
		"BPF_SKB_LOAD_BYTES_RELATIVE":        BpfSkbLoadBytesRelative,
		"BPF_FIB_LOOKUP":                     BpfFibLookup,
		"BPF_SOCK_HASH_UPDATE":               BpfSockHashUpdate,
		"BPF_MSG_REDIRECT_HASH":              BpfMsgRedirectHash,
		"BPF_SK_REDIRECT_HASH":               BpfSkRedirectHash,
		"BPF_LWT_PUSH_ENCAP":                 BpfLwtPushEncap,
		"BPF_LWT_SEG6_STORE_BYTES":           BpfLwtSeg6StoreBytes,
		"BPF_LWT_SEG6_ADJUST_SRH":            BpfLwtSeg6AdjustSrh,
		"BPF_LWT_SEG6_ACTION":                BpfLwtSeg6Action,
		"BPF_RC_REPEAT":                      BpfRcRepeat,
		"BPF_RC_KEYDOWN":                     BpfRcKeydown,
		"BPF_SKB_CGROUP_ID":                  BpfSkbCgroupID,
		"BPF_GET_CURRENT_CGROUP_ID":          BpfGetCurrentCgroupID,
		"BPF_GET_LOCAL_STORAGE":              BpfGetLocalStorage,
		"BPF_SK_SELECT_REUSEPORT":            BpfSkSelectReuseport,
		"BPF_SKB_ANCESTOR_CGROUP_ID":         BpfSkbAncestorCgroupID,
		"BPF_SK_LOOKUP_TCP":                  BpfSkLookupTCP,
		"BPF_SK_LOOKUP_UDP":                  BpfSkLookupUDP,
		"BPF_SK_RELEASE":                     BpfSkRelease,
		"BPF_MAP_PUSH_ELEM":                  BpfMapPushElem,
		"BPF_MAP_POP_ELEM":                   BpfMapPopElem,
		"BPF_MAP_PEEK_ELEM":                  BpfMapPeekElem,
		"BPF_MSG_PUSH_DATA":                  BpfMsgPushData,
		"BPF_MSG_POP_DATA":                   BpfMsgPopData,
		"BPF_RC_POINTER_REL":                 BpfRcPointerRel,
		"BPF_SPIN_LOCK":                      BpfSpinLock,
		"BPF_SPIN_UNLOCK":                    BpfSpinUnlock,
		"BPF_SK_FULLSOCK":                    BpfSkFullsock,
		"BPF_TCP_SOCK":                       BpfTCPSock,
		"BPF_SKB_ECN_SET_CE":                 BpfSkbEcnSetCe,
		"BPF_GET_LISTENER_SOCK":              BpfGetListenerSock,
		"BPF_SKC_LOOKUP_TCP":                 BpfSkcLookupTCP,
		"BPF_TCP_CHECK_SYNCOOKIE":            BpfTCPCheckSyncookie,
		"BPF_SYSCTL_GET_NAME":                BpfSysctlGetName,
		"BPF_SYSCTL_GET_CURRENT_VALUE":       BpfSysctlGetCurrentValue,
		"BPF_SYSCTL_GET_NEW_VALUE":           BpfSysctlGetNewValue,
		"BPF_SYSCTL_SET_NEW_VALUE":           BpfSysctlSetNewValue,
		"BPF_STRTOL":                         BpfStrtol,
		"BPF_STRTOUL":                        BpfStrtoul,
		"BPF_SK_STORAGE_GET":                 BpfSkStorageGet,
		"BPF_SK_STORAGE_DELETE":              BpfSkStorageDelete,
		"BPF_SEND_SIGNAL":                    BpfSendSignal,
		"BPF_TCP_GEN_SYNCOOKIE":              BpfTCPGenSyncookie,
		"BPF_SKB_OUTPUT":                     BpfSkbOutput,
		"BPF_PROBE_READ_USER":                BpfProbeReadUser,
		"BPF_PROBE_READ_KERNEL":              BpfProbeReadKernel,
		"BPF_PROBE_READ_USER_STR":            BpfProbeReadUserStr,
		"BPF_PROBE_READ_KERNEL_STR":          BpfProbeReadKernelStr,
		"BPF_TCP_SEND_ACK":                   BpfTCPSendAck,
		"BPF_SEND_SIGNAL_THREAD":             BpfSendSignalThread,
		"BPF_JIFFIES64":                      BpfJiffies64,
		"BPF_READ_BRANCH_RECORDS":            BpfReadBranchRecords,
		"BPF_GET_NS_CURRENT_PID_TGID":        BpfGetNsCurrentPidTgid,
		"BPF_XDP_OUTPUT":                     BpfXdpOutput,
		"BPF_GET_NETNS_COOKIE":               BpfGetNetnsCookie,
		"BPF_GET_CURRENT_ANCESTOR_CGROUP_ID": BpfGetCurrentAncestorCgroupID,
		"BPF_SK_ASSIGN":                      BpfSkAssign,
		"BPF_KTIME_GET_BOOT_NS":              BpfKtimeGetBootNs,
		"BPF_SEQ_PRINTF":                     BpfSeqPrintf,
		"BPF_SEQ_WRITE":                      BpfSeqWrite,
		"BPF_SK_CGROUP_ID":                   BpfSkCgroupID,
		"BPF_SK_ANCESTOR_CGROUP_ID":          BpfSkAncestorCgroupID,
		"BPF_RINGBUF_OUTPUT":                 BpfRingbufOutput,
		"BPF_RINGBUF_RESERVE":                BpfRingbufReserve,
		"BPF_RINGBUF_SUBMIT":                 BpfRingbufSubmit,
		"BPF_RINGBUF_DISCARD":                BpfRingbufDiscard,
		"BPF_RINGBUF_QUERY":                  BpfRingbufQuery,
		"BPF_CSUM_LEVEL":                     BpfCsumLevel,
		"BPF_SKC_TO_TCP6_SOCK":               BpfSkcToTCP6Sock,
		"BPF_SKC_TO_TCP_SOCK":                BpfSkcToTCPSock,
		"BPF_SKC_TO_TCP_TIMEWAIT_SOCK":       BpfSkcToTCPTimewaitSock,
		"BPF_SKC_TO_TCP_REQUEST_SOCK":        BpfSkcToTCPRequestSock,
		"BPF_SKC_TO_UDP6_SOCK":               BpfSkcToUDP6Sock,
		"BPF_GET_TASK_STACK":                 BpfGetTaskStack,
		"BPF_LOAD_HDR_OPT":                   BpfLoadHdrOpt,
		"BPF_STORE_HDR_OPT":                  BpfStoreHdrOpt,
		"BPF_RESERVE_HDR_OPT":                BpfReserveHdrOpt,
		"BPF_INODE_STORAGE_GET":              BpfInodeStorageGet,
		"BPF_INODE_STORAGE_DELETE":           BpfInodeStorageDelete,
		"BPF_D_PATH":                         BpfDPath,
		"BPF_COPY_FROM_USER":                 BpfCopyFromUser,
		"BPF_SNPRINTF_BTF":                   BpfSnprintfBtf,
		"BPF_SEQ_PRINTF_BTF":                 BpfSeqPrintfBtf,
		"BPF_SKB_CGROUP_CLASSID":             BpfSkbCgroupClassid,
		"BPF_REDIRECT_NEIGH":                 BpfRedirectNeigh,
		"BPF_PER_CPU_PTR":                    BpfPerCPUPtr,
		"BPF_THIS_CPU_PTR":                   BpfThisCPUPtr,
		"BPF_REDIRECT_PEER":                  BpfRedirectPeer,
		"BPF_TASK_STORAGE_GET":               BpfTaskStorageGet,
		"BPF_TASK_STORAGE_DELETE":            BpfTaskStorageDelete,
		"BPF_GET_CURRENT_TASK_BTF":           BpfGetCurrentTaskBtf,
		"BPF_BPRM_OPTS_SET":                  BpfBprmOptsSet,
		"BPF_KTIME_GET_COARSE_NS":            BpfKtimeGetCoarseNs,
		"BPF_IMA_INODE_HASH":                 BpfImaInodeHash,
		"BPF_SOCK_FROM_FILE":                 BpfSockFromFile,
		"BPF_CHECK_MTU":                      BpfCheckMtu,
		"BPF_FOR_EACH_MAP_ELEM":              BpfForEachMapElem,
		"BPF_SNPRINTF":                       BpfSnprintf,
	}

	// BPFMapTypeConstants is the list of BPF map type constants
	// generate_constants:BPF map types,BPF map types are the supported eBPF map types.
	BPFMapTypeConstants = map[string]BPFMapType{
		"BPF_MAP_TYPE_UNSPEC":                BpfMapTypeUnspec,
		"BPF_MAP_TYPE_HASH":                  BpfMapTypeHash,
		"BPF_MAP_TYPE_ARRAY":                 BpfMapTypeArray,
		"BPF_MAP_TYPE_PROG_ARRAY":            BpfMapTypeProgArray,
		"BPF_MAP_TYPE_PERF_EVENT_ARRAY":      BpfMapTypePerfEventArray,
		"BPF_MAP_TYPE_PERCPU_HASH":           BpfMapTypePercpuHash,
		"BPF_MAP_TYPE_PERCPU_ARRAY":          BpfMapTypePercpuArray,
		"BPF_MAP_TYPE_STACK_TRACE":           BpfMapTypeStackTrace,
		"BPF_MAP_TYPE_CGROUP_ARRAY":          BpfMapTypeCgroupArray,
		"BPF_MAP_TYPE_LRU_HASH":              BpfMapTypeLruHash,
		"BPF_MAP_TYPE_LRU_PERCPU_HASH":       BpfMapTypeLruPercpuHash,
		"BPF_MAP_TYPE_LPM_TRIE":              BpfMapTypeLpmTrie,
		"BPF_MAP_TYPE_ARRAY_OF_MAPS":         BpfMapTypeArrayOfMaps,
		"BPF_MAP_TYPE_HASH_OF_MAPS":          BpfMapTypeHashOfMaps,
		"BPF_MAP_TYPE_DEVMAP":                BpfMapTypeDevmap,
		"BPF_MAP_TYPE_SOCKMAP":               BpfMapTypeSockmap,
		"BPF_MAP_TYPE_CPUMAP":                BpfMapTypeCPUmap,
		"BPF_MAP_TYPE_XSKMAP":                BpfMapTypeXskmap,
		"BPF_MAP_TYPE_SOCKHASH":              BpfMapTypeSockhash,
		"BPF_MAP_TYPE_CGROUP_STORAGE":        BpfMapTypeCgroupStorage,
		"BPF_MAP_TYPE_REUSEPORT_SOCKARRAY":   BpfMapTypeReuseportSockarray,
		"BPF_MAP_TYPE_PERCPU_CGROUP_STORAGE": BpfMapTypePercpuCgroupStorage,
		"BPF_MAP_TYPE_QUEUE":                 BpfMapTypeQueue,
		"BPF_MAP_TYPE_STACK":                 BpfMapTypeStack,
		"BPF_MAP_TYPE_SK_STORAGE":            BpfMapTypeSkStorage,
		"BPF_MAP_TYPE_DEVMAP_HASH":           BpfMapTypeDevmapHash,
		"BPF_MAP_TYPE_STRUCT_OPS":            BpfMapTypeStructOps,
		"BPF_MAP_TYPE_RINGBUF":               BpfMapTypeRingbuf,
		"BPF_MAP_TYPE_INODE_STORAGE":         BpfMapTypeInodeStorage,
		"BPF_MAP_TYPE_TASK_STORAGE":          BpfMapTypeTaskStorage,
	}

	// BPFProgramTypeConstants is the list of BPF program type constants
	// generate_constants:BPF program types,BPF program types are the supported eBPF program types.
	BPFProgramTypeConstants = map[string]BPFProgramType{
		"BPF_PROG_TYPE_UNSPEC":                  BpfProgTypeUnspec,
		"BPF_PROG_TYPE_SOCKET_FILTER":           BpfProgTypeSocketFilter,
		"BPF_PROG_TYPE_KPROBE":                  BpfProgTypeKprobe,
		"BPF_PROG_TYPE_SCHED_CLS":               BpfProgTypeSchedCls,
		"BPF_PROG_TYPE_SCHED_ACT":               BpfProgTypeSchedAct,
		"BPF_PROG_TYPE_TRACEPOINT":              BpfProgTypeTracepoint,
		"BPF_PROG_TYPE_XDP":                     BpfProgTypeXdp,
		"BPF_PROG_TYPE_PERF_EVENT":              BpfProgTypePerfEvent,
		"BPF_PROG_TYPE_CGROUP_SKB":              BpfProgTypeCgroupSkb,
		"BPF_PROG_TYPE_CGROUP_SOCK":             BpfProgTypeCgroupSock,
		"BPF_PROG_TYPE_LWT_IN":                  BpfProgTypeLwtIn,
		"BPF_PROG_TYPE_LWT_OUT":                 BpfProgTypeLwtOut,
		"BPF_PROG_TYPE_LWT_XMIT":                BpfProgTypeLwtXmit,
		"BPF_PROG_TYPE_SOCK_OPS":                BpfProgTypeSockOps,
		"BPF_PROG_TYPE_SK_SKB":                  BpfProgTypeSkSkb,
		"BPF_PROG_TYPE_CGROUP_DEVICE":           BpfProgTypeCgroupDevice,
		"BPF_PROG_TYPE_SK_MSG":                  BpfProgTypeSkMsg,
		"BPF_PROG_TYPE_RAW_TRACEPOINT":          BpfProgTypeRawTracepoint,
		"BPF_PROG_TYPE_CGROUP_SOCK_ADDR":        BpfProgTypeCgroupSockAddr,
		"BPF_PROG_TYPE_LWT_SEG6LOCAL":           BpfProgTypeLwtSeg6local,
		"BPF_PROG_TYPE_LIRC_MODE2":              BpfProgTypeLircMode2,
		"BPF_PROG_TYPE_SK_REUSEPORT":            BpfProgTypeSkReuseport,
		"BPF_PROG_TYPE_FLOW_DISSECTOR":          BpfProgTypeFlowDissector,
		"BPF_PROG_TYPE_CGROUP_SYSCTL":           BpfProgTypeCgroupSysctl,
		"BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE": BpfProgTypeRawTracepointWritable,
		"BPF_PROG_TYPE_CGROUP_SOCKOPT":          BpfProgTypeCgroupSockopt,
		"BPF_PROG_TYPE_TRACING":                 BpfProgTypeTracing,
		"BPF_PROG_TYPE_STRUCT_OPS":              BpfProgTypeStructOps,
		"BPF_PROG_TYPE_EXT":                     BpfProgTypeExt,
		"BPF_PROG_TYPE_LSM":                     BpfProgTypeLsm,
		"BPF_PROG_TYPE_SK_LOOKUP":               BpfProgTypeSkLookup,
	}

	// BPFAttachTypeConstants is the list of BPF attach type constants
	// generate_constants:BPF attach types,BPF attach types are the supported eBPF program attach types.
	BPFAttachTypeConstants = map[string]BPFAttachType{
		"BPF_CGROUP_INET_INGRESS":      BpfCgroupInetIngress,
		"BPF_CGROUP_INET_EGRESS":       BpfCgroupInetEgress,
		"BPF_CGROUP_INET_SOCK_CREATE":  BpfCgroupInetSockCreate,
		"BPF_CGROUP_SOCK_OPS":          BpfCgroupSockOps,
		"BPF_SK_SKB_STREAM_PARSER":     BpfSkSkbStreamParser,
		"BPF_SK_SKB_STREAM_VERDICT":    BpfSkSkbStreamVerdict,
		"BPF_CGROUP_DEVICE":            BpfCgroupDevice,
		"BPF_SK_MSG_VERDICT":           BpfSkMsgVerdict,
		"BPF_CGROUP_INET4_BIND":        BpfCgroupInet4Bind,
		"BPF_CGROUP_INET6_BIND":        BpfCgroupInet6Bind,
		"BPF_CGROUP_INET4_CONNECT":     BpfCgroupInet4Connect,
		"BPF_CGROUP_INET6_CONNECT":     BpfCgroupInet6Connect,
		"BPF_CGROUP_INET4_POST_BIND":   BpfCgroupInet4PostBind,
		"BPF_CGROUP_INET6_POST_BIND":   BpfCgroupInet6PostBind,
		"BPF_CGROUP_UDP4_SENDMSG":      BpfCgroupUDP4Sendmsg,
		"BPF_CGROUP_UDP6_SENDMSG":      BpfCgroupUDP6Sendmsg,
		"BPF_LIRC_MODE2":               BpfLircMode2,
		"BPF_FLOW_DISSECTOR":           BpfFlowDissector,
		"BPF_CGROUP_SYSCTL":            BpfCgroupSysctl,
		"BPF_CGROUP_UDP4_RECVMSG":      BpfCgroupUDP4Recvmsg,
		"BPF_CGROUP_UDP6_RECVMSG":      BpfCgroupUDP6Recvmsg,
		"BPF_CGROUP_GETSOCKOPT":        BpfCgroupGetsockopt,
		"BPF_CGROUP_SETSOCKOPT":        BpfCgroupSetsockopt,
		"BPF_TRACE_RAW_TP":             BpfTraceRawTp,
		"BPF_TRACE_FENTRY":             BpfTraceFentry,
		"BPF_TRACE_FEXIT":              BpfTraceFexit,
		"BPF_MODIFY_RETURN":            BpfModifyReturn,
		"BPF_LSM_MAC":                  BpfLsmMac,
		"BPF_TRACE_ITER":               BpfTraceIter,
		"BPF_CGROUP_INET4_GETPEERNAME": BpfCgroupInet4Getpeername,
		"BPF_CGROUP_INET6_GETPEERNAME": BpfCgroupInet6Getpeername,
		"BPF_CGROUP_INET4_GETSOCKNAME": BpfCgroupInet4Getsockname,
		"BPF_CGROUP_INET6_GETSOCKNAME": BpfCgroupInet6Getsockname,
		"BPF_XDP_DEVMAP":               BpfXdpDevmap,
		"BPF_CGROUP_INET_SOCK_RELEASE": BpfCgroupInetSockRelease,
		"BPF_XDP_CPUMAP":               BpfXdpCPUmap,
		"BPF_SK_LOOKUP":                BpfSkLookup,
		"BPF_XDP":                      BpfXdp,
		"BPF_SK_SKB_VERDICT":           BpfSkSkbVerdict,
	}

	// PipeBufFlagConstants is the list of pipe buffer flags
	// generate_constants:Pipe buffer flags,Pipe buffer flags are the supported flags for a pipe buffer.
	PipeBufFlagConstants = map[string]PipeBufFlag{
		"PIPE_BUF_FLAG_LRU":       PipeBufFlagLRU,
		"PIPE_BUF_FLAG_ATOMIC":    PipeBufFlagAtomic,
		"PIPE_BUF_FLAG_GIFT":      PipeBufFlagGift,
		"PIPE_BUF_FLAG_PACKET":    PipeBufFlagPacket,
		"PIPE_BUF_FLAG_CAN_MERGE": PipeBufFlagCanMerge,
		"PIPE_BUF_FLAG_WHOLE":     PipeBufFlagWhole,
		"PIPE_BUF_FLAG_LOSS":      PipeBufFlagLoss,
	}

	// DNSQTypeConstants see https://www.iana.org/assignments/dns-parameters/dns-parameters.xhtml
	// generate_constants:DNS qtypes,DNS qtypes are the supported DNS query types.
	DNSQTypeConstants = map[string]int{
		"None":       0,
		"A":          1,
		"NS":         2,
		"MD":         3,
		"MF":         4,
		"CNAME":      5,
		"SOA":        6,
		"MB":         7,
		"MG":         8,
		"MR":         9,
		"NULL":       10,
		"PTR":        12,
		"HINFO":      13,
		"MINFO":      14,
		"MX":         15,
		"TXT":        16,
		"RP":         17,
		"AFSDB":      18,
		"X25":        19,
		"ISDN":       20,
		"RT":         21,
		"NSAPPTR":    23,
		"SIG":        24,
		"KEY":        25,
		"PX":         26,
		"GPOS":       27,
		"AAAA":       28,
		"LOC":        29,
		"NXT":        30,
		"EID":        31,
		"NIMLOC":     32,
		"SRV":        33,
		"ATMA":       34,
		"NAPTR":      35,
		"KX":         36,
		"CERT":       37,
		"DNAME":      39,
		"OPT":        41,
		"APL":        42,
		"DS":         43,
		"SSHFP":      44,
		"RRSIG":      46,
		"NSEC":       47,
		"DNSKEY":     48,
		"DHCID":      49,
		"NSEC3":      50,
		"NSEC3PARAM": 51,
		"TLSA":       52,
		"SMIMEA":     53,
		"HIP":        55,
		"NINFO":      56,
		"RKEY":       57,
		"TALINK":     58,
		"CDS":        59,
		"CDNSKEY":    60,
		"OPENPGPKEY": 61,
		"CSYNC":      62,
		"ZONEMD":     63,
		"SVCB":       64,
		"HTTPS":      65,
		"SPF":        99,
		"UINFO":      100,
		"UID":        101,
		"GID":        102,
		"UNSPEC":     103,
		"NID":        104,
		"L32":        105,
		"L64":        106,
		"LP":         107,
		"EUI48":      108,
		"EUI64":      109,
		"URI":        256,
		"CAA":        257,
		"AVC":        258,
		"TKEY":       249,
		"TSIG":       250,
		"IXFR":       251,
		"AXFR":       252,
		"MAILB":      253,
		"MAILA":      254,
		"ANY":        255,
		"TA":         32768,
		"DLV":        32769,
		"Reserved":   65535,
	}

	// DNSQClassConstants see https://www.iana.org/assignments/dns-parameters/dns-parameters.xhtml
	// generate_constants:DNS qclasses,DNS qclasses are the supported DNS query classes.
	DNSQClassConstants = map[string]int{
		"CLASS_INET":   1,
		"CLASS_CSNET":  2,
		"CLASS_CHAOS":  3,
		"CLASS_HESIOD": 4,
		"CLASS_NONE":   254,
		"CLASS_ANY":    255,
	}

	// SECLConstants are constants supported in runtime security agent rules
	// generate_constants:SecL constants,SecL constants are the supported generic SecL constants.
	SECLConstants = map[string]interface{}{
		// boolean
		"true":  &eval.BoolEvaluator{Value: true},
		"false": &eval.BoolEvaluator{Value: false},
	}

	// L3ProtocolConstants is the list of supported L3 protocols
	// generate_constants:L3 protocols,L3 protocols are the supported Layer 3 protocols.
	L3ProtocolConstants = map[string]L3Protocol{
		"ETH_P_LOOP":            EthPLOOP,
		"ETH_P_PUP":             EthPPUP,
		"ETH_P_PUPAT":           EthPPUPAT,
		"ETH_P_TSN":             EthPTSN,
		"ETH_P_IP":              EthPIP,
		"ETH_P_X25":             EthPX25,
		"ETH_P_ARP":             EthPARP,
		"ETH_P_BPQ":             EthPBPQ,
		"ETH_P_IEEEPUP":         EthPIEEEPUP,
		"ETH_P_IEEEPUPAT":       EthPIEEEPUPAT,
		"ETH_P_BATMAN":          EthPBATMAN,
		"ETH_P_DEC":             EthPDEC,
		"ETH_P_DNADL":           EthPDNADL,
		"ETH_P_DNARC":           EthPDNARC,
		"ETH_P_DNART":           EthPDNART,
		"ETH_P_LAT":             EthPLAT,
		"ETH_P_DIAG":            EthPDIAG,
		"ETH_P_CUST":            EthPCUST,
		"ETH_P_SCA":             EthPSCA,
		"ETH_P_TEB":             EthPTEB,
		"ETH_P_RARP":            EthPRARP,
		"ETH_P_ATALK":           EthPATALK,
		"ETH_P_AARP":            EthPAARP,
		"ETH_P_8021_Q":          EthP8021Q,
		"ETH_P_ERSPAN":          EthPERSPAN,
		"ETH_P_IPX":             EthPIPX,
		"ETH_P_IPV6":            EthPIPV6,
		"ETH_P_PAUSE":           EthPPAUSE,
		"ETH_P_SLOW":            EthPSLOW,
		"ETH_P_WCCP":            EthPWCCP,
		"ETH_P_MPLSUC":          EthPMPLSUC,
		"ETH_P_MPLSMC":          EthPMPLSMC,
		"ETH_P_ATMMPOA":         EthPATMMPOA,
		"ETH_P_PPPDISC":         EthPPPPDISC,
		"ETH_P_PPPSES":          EthPPPPSES,
		"ETH_P__LINK_CTL":       EthPLinkCTL,
		"ETH_P_ATMFATE":         EthPATMFATE,
		"ETH_P_PAE":             EthPPAE,
		"ETH_P_AOE":             EthPAOE,
		"ETH_P_8021_AD":         EthP8021AD,
		"ETH_P_802_EX1":         EthP802EX1,
		"ETH_P_TIPC":            EthPTIPC,
		"ETH_P_MACSEC":          EthPMACSEC,
		"ETH_P_8021_AH":         EthP8021AH,
		"ETH_P_MVRP":            EthPMVRP,
		"ETH_P_1588":            EthP1588,
		"ETH_P_NCSI":            EthPNCSI,
		"ETH_P_PRP":             EthPPRP,
		"ETH_P_FCOE":            EthPFCOE,
		"ETH_P_IBOE":            EthPIBOE,
		"ETH_P_TDLS":            EthPTDLS,
		"ETH_P_FIP":             EthPFIP,
		"ETH_P_80221":           EthP80221,
		"ETH_P_HSR":             EthPHSR,
		"ETH_P_NSH":             EthPNSH,
		"ETH_P_LOOPBACK":        EthPLOOPBACK,
		"ETH_P_QINQ1":           EthPQINQ1,
		"ETH_P_QINQ2":           EthPQINQ2,
		"ETH_P_QINQ3":           EthPQINQ3,
		"ETH_P_EDSA":            EthPEDSA,
		"ETH_P_IFE":             EthPIFE,
		"ETH_P_AFIUCV":          EthPAFIUCV,
		"ETH_P_8023_MIN":        EthP8023MIN,
		"ETH_P_IPV6_HOP_BY_HOP": EthPIPV6HopByHop,
		"ETH_P_8023":            EthP8023,
		"ETH_P_AX25":            EthPAX25,
		"ETH_P_ALL":             EthPALL,
		"ETH_P_8022":            EthP8022,
		"ETH_P_SNAP":            EthPSNAP,
		"ETH_P_DDCMP":           EthPDDCMP,
		"ETH_P_WANPPP":          EthPWANPPP,
		"ETH_P_PPPMP":           EthPPPPMP,
		"ETH_P_LOCALTALK":       EthPLOCALTALK,
		"ETH_P_CAN":             EthPCAN,
		"ETH_P_CANFD":           EthPCANFD,
		"ETH_P_PPPTALK":         EthPPPPTALK,
		"ETH_P_TR8022":          EthPTR8022,
		"ETH_P_MOBITEX":         EthPMOBITEX,
		"ETH_P_CONTROL":         EthPCONTROL,
		"ETH_P_IRDA":            EthPIRDA,
		"ETH_P_ECONET":          EthPECONET,
		"ETH_P_HDLC":            EthPHDLC,
		"ETH_P_ARCNET":          EthPARCNET,
		"ETH_P_DSA":             EthPDSA,
		"ETH_P_TRAILER":         EthPTRAILER,
		"ETH_P_PHONET":          EthPPHONET,
		"ETH_P_IEEE802154":      EthPIEEE802154,
		"ETH_P_CAIF":            EthPCAIF,
		"ETH_P_XDSA":            EthPXDSA,
		"ETH_P_MAP":             EthPMAP,
	}

	// L4ProtocolConstants is the list of supported L4 protocols
	// generate_constants:L4 protocols,L4 protocols are the supported Layer 4 protocols.
	L4ProtocolConstants = map[string]L4Protocol{
		"IP_PROTO_IP":      IPProtoIP,
		"IP_PROTO_ICMP":    IPProtoICMP,
		"IP_PROTO_IGMP":    IPProtoIGMP,
		"IP_PROTO_IPIP":    IPProtoIPIP,
		"IP_PROTO_TCP":     IPProtoTCP,
		"IP_PROTO_EGP":     IPProtoEGP,
		"IP_PROTO_IGP":     IPProtoIGP,
		"IP_PROTO_PUP":     IPProtoPUP,
		"IP_PROTO_UDP":     IPProtoUDP,
		"IP_PROTO_IDP":     IPProtoIDP,
		"IP_PROTO_TP":      IPProtoTP,
		"IP_PROTO_DCCP":    IPProtoDCCP,
		"IP_PROTO_IPV6":    IPProtoIPV6,
		"IP_PROTO_RSVP":    IPProtoRSVP,
		"IP_PROTO_GRE":     IPProtoGRE,
		"IP_PROTO_ESP":     IPProtoESP,
		"IP_PROTO_AH":      IPProtoAH,
		"IP_PROTO_ICMPV6":  IPProtoICMPV6,
		"IP_PROTO_MTP":     IPProtoMTP,
		"IP_PROTO_BEETPH":  IPProtoBEETPH,
		"IP_PROTO_ENCAP":   IPProtoENCAP,
		"IP_PROTO_PIM":     IPProtoPIM,
		"IP_PROTO_COMP":    IPProtoCOMP,
		"IP_PROTO_SCTP":    IPProtoSCTP,
		"IP_PROTO_UDPLITE": IPProtoUDPLITE,
		"IP_PROTO_MPLS":    IPProtoMPLS,
		"IP_PROTO_RAW":     IPProtoRAW,
	}

	// exitCauseConstants is the list of supported Exit causes
	exitCauseConstants = map[string]ExitCause{
		"EXITED":     ExitExited,
		"COREDUMPED": ExitCoreDumped,
		"SIGNALED":   ExitSignaled,
	}
)

var (
	openFlagsStrings          = map[int]string{}
	chmodModeStrings          = map[int]string{}
	unlinkFlagsStrings        = map[int]string{}
	kernelCapabilitiesStrings = map[uint64]string{}
	bpfCmdStrings             = map[uint32]string{}
	bpfHelperFuncStrings      = map[uint32]string{}
	bpfMapTypeStrings         = map[uint32]string{}
	bpfProgramTypeStrings     = map[uint32]string{}
	bpfAttachTypeStrings      = map[uint32]string{}
	ptraceFlagsStrings        = map[uint32]string{}
	vmStrings                 = map[int]string{}
	protStrings               = map[int]string{}
	mmapFlagStrings           = map[int]string{}
	signalStrings             = map[int]string{}
	pipeBufFlagStrings        = map[int]string{}
	dnsQTypeStrings           = map[uint32]string{}
	dnsQClassStrings          = map[uint32]string{}
	l3ProtocolStrings         = map[L3Protocol]string{}
	l4ProtocolStrings         = map[L4Protocol]string{}
	addressFamilyStrings      = map[uint16]string{}
	exitCauseStrings          = map[ExitCause]string{}
)

// File flags
const (
	LowerLayer = 1 << iota
	UpperLayer
)

func initOpenConstants() {
	for k, v := range openFlagsConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}

	for k, v := range openFlagsConstants {
		openFlagsStrings[v] = k
	}
}

func initChmodConstants() {
	for k, v := range chmodModeConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
		chmodModeStrings[v] = k
	}
}

func initUnlinkConstanst() {
	for k, v := range unlinkFlagsConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
		unlinkFlagsStrings[v] = k
	}
}

func initErrorConstants() {
	for k, v := range errorConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}
}

func initKernelCapabilityConstants() {
	for k, v := range KernelCapabilityConstants {
		if bits.UintSize == 64 || v < math.MaxInt32 {
			SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		}
		kernelCapabilitiesStrings[v] = k
	}
}

func initBPFCmdConstants() {
	for k, v := range BPFCmdConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfCmdStrings[uint32(v)] = k
	}
}

func initBPFHelperFuncConstants() {
	for k, v := range BPFHelperFuncConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfHelperFuncStrings[uint32(v)] = k
	}
}

func initBPFMapTypeConstants() {
	for k, v := range BPFMapTypeConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfMapTypeStrings[uint32(v)] = k
	}
}

func initBPFProgramTypeConstants() {
	for k, v := range BPFProgramTypeConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfProgramTypeStrings[uint32(v)] = k
	}
}

func initBPFAttachTypeConstants() {
	for k, v := range BPFAttachTypeConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfAttachTypeStrings[uint32(v)] = k
	}
}

func initPtraceConstants() {
	for k, v := range ptraceArchConstants {
		ptraceConstants[k] = v
	}

	for k, v := range ptraceConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
	}

	for k, v := range ptraceConstants {
		ptraceFlagsStrings[v] = k
	}
}

func initVMConstants() {
	for k, v := range vmConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}

	for k, v := range vmConstants {
		vmStrings[v] = k
	}
}

func initProtConstansts() {
	for k, v := range protConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}

	for k, v := range protConstants {
		protStrings[v] = k
	}
}

func initMMapFlagsConstants() {
	for k, v := range mmapFlagArchConstants {
		mmapFlagConstants[k] = v
	}

	for k, v := range mmapFlagConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}

	for k, v := range mmapFlagConstants {
		mmapFlagStrings[v] = k
	}
}

func initSignalConstants() {
	for k, v := range signalConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}

	for k, v := range signalConstants {
		signalStrings[v] = k
	}
}

func initPipeBufFlagConstants() {
	for k, v := range PipeBufFlagConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		pipeBufFlagStrings[int(v)] = k
	}
}

func initDNSQClassConstants() {
	for k, v := range DNSQClassConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
		dnsQClassStrings[uint32(v)] = k
	}
}

func initDNSQTypeConstants() {
	for k, v := range DNSQTypeConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
		dnsQTypeStrings[uint32(v)] = k
	}
}

func initL3ProtocolConstants() {
	for k, v := range L3ProtocolConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		l3ProtocolStrings[v] = k
	}
}

func initL4ProtocolConstants() {
	for k, v := range L4ProtocolConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		l4ProtocolStrings[v] = k
	}
}

func initAddressFamilyConstants() {
	for k, v := range addressFamilyConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
	}

	for k, v := range addressFamilyConstants {
		addressFamilyStrings[v] = k
	}
}

func initExitCauseConstants() {
	for k, v := range exitCauseConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: int(v)}
		exitCauseStrings[v] = k
	}
}

func initConstants() {
	initErrorConstants()
	initOpenConstants()
	initChmodConstants()
	initUnlinkConstanst()
	initKernelCapabilityConstants()
	initBPFCmdConstants()
	initBPFHelperFuncConstants()
	initBPFMapTypeConstants()
	initBPFProgramTypeConstants()
	initBPFAttachTypeConstants()
	initPtraceConstants()
	initVMConstants()
	initProtConstansts()
	initMMapFlagsConstants()
	initSignalConstants()
	initPipeBufFlagConstants()
	initDNSQClassConstants()
	initDNSQTypeConstants()
	initL3ProtocolConstants()
	initL4ProtocolConstants()
	initAddressFamilyConstants()
	initExitCauseConstants()
}

func bitmaskToStringArray(bitmask int, intToStrMap map[int]string) []string {
	var strs []string
	var result int

	for v, s := range intToStrMap {
		if v == 0 {
			continue
		}

		if bitmask&v == v {
			strs = append(strs, s)
			result |= v
		}
	}

	if result != bitmask {
		strs = append(strs, fmt.Sprintf("%d", bitmask&^result))
	}

	sort.Strings(strs)
	return strs
}

func bitmaskToString(bitmask int, intToStrMap map[int]string) string {
	return strings.Join(bitmaskToStringArray(bitmask, intToStrMap), " | ")
}

func bitmaskU64ToStringArray(bitmask uint64, intToStrMap map[uint64]string) []string {
	var strs []string
	var result uint64

	for v, s := range intToStrMap {
		if v == 0 {
			continue
		}

		if bitmask&v == v {
			strs = append(strs, s)
			result |= v
		}
	}

	if result != bitmask {
		strs = append(strs, fmt.Sprintf("%d", bitmask&^result))
	}

	sort.Strings(strs)
	return strs
}

func bitmaskU64ToString(bitmask uint64, intToStrMap map[uint64]string) string {
	return strings.Join(bitmaskU64ToStringArray(bitmask, intToStrMap), " | ")
}

// OpenFlags represents an open flags bitmask value
type OpenFlags int

func (f OpenFlags) String() string {
	if int(f) == syscall.O_RDONLY {
		return openFlagsStrings[syscall.O_RDONLY]
	}
	return bitmaskToString(int(f), openFlagsStrings)
}

// StringArray returns the open flags as an array of strings
func (f OpenFlags) StringArray() []string {
	if int(f) == syscall.O_RDONLY {
		return []string{openFlagsStrings[syscall.O_RDONLY]}
	}
	return bitmaskToStringArray(int(f), openFlagsStrings)
}

// ChmodMode represent a chmod mode bitmask value
type ChmodMode int

func (m ChmodMode) String() string {
	return bitmaskToString(int(m), chmodModeStrings)
}

// UnlinkFlags represents an unlink flags bitmask value
type UnlinkFlags int

func (f UnlinkFlags) String() string {
	return bitmaskToString(int(f), unlinkFlagsStrings)
}

// StringArray returns the unlink flags as an array of strings
func (f UnlinkFlags) StringArray() []string {
	return bitmaskToStringArray(int(f), unlinkFlagsStrings)
}

// RetValError represents a syscall return error value
type RetValError int

func (f RetValError) String() string {
	v := int(f)
	if v < 0 {
		return syscall.Errno(-v).Error()
	}
	return ""
}

var capsStringArrayCache *lru.Cache

func init() {
	initConstants()
	capsStringArrayCache, _ = lru.New(4)
}

// KernelCapability represents a kernel capability bitmask value
type KernelCapability uint64

func (kc KernelCapability) String() string {
	return bitmaskU64ToString(uint64(kc), kernelCapabilitiesStrings)
}

// StringArray returns the kernel capabilities as an array of strings
func (kc KernelCapability) StringArray() []string {
	if value, ok := capsStringArrayCache.Get(kc); ok {
		return value.([]string)
	}
	computed := bitmaskU64ToStringArray(uint64(kc), kernelCapabilitiesStrings)
	capsStringArrayCache.Add(kc, computed)
	return computed
}

// BPFCmd represents a BPF command
type BPFCmd uint64

func (cmd BPFCmd) String() string {
	return bpfCmdStrings[uint32(cmd)]
}

const (
	// BpfMapCreateCmd command
	BpfMapCreateCmd BPFCmd = iota
	// BpfMapLookupElemCmd command
	BpfMapLookupElemCmd
	// BpfMapUpdateElemCmd command
	BpfMapUpdateElemCmd
	// BpfMapDeleteElemCmd command
	BpfMapDeleteElemCmd
	// BpfMapGetNextKeyCmd command
	BpfMapGetNextKeyCmd
	// BpfProgLoadCmd command
	BpfProgLoadCmd
	// BpfObjPinCmd command
	BpfObjPinCmd
	// BpfObjGetCmd command
	BpfObjGetCmd
	// BpfProgAttachCmd command
	BpfProgAttachCmd
	// BpfProgDetachCmd command
	BpfProgDetachCmd
	// BpfProgTestRunCmd command
	BpfProgTestRunCmd
	// BpfProgGetNextIDCmd command
	BpfProgGetNextIDCmd
	// BpfMapGetNextIDCmd command
	BpfMapGetNextIDCmd
	// BpfProgGetFdByIDCmd command
	BpfProgGetFdByIDCmd
	// BpfMapGetFdByIDCmd command
	BpfMapGetFdByIDCmd
	// BpfObjGetInfoByFdCmd command
	BpfObjGetInfoByFdCmd
	// BpfProgQueryCmd command
	BpfProgQueryCmd
	// BpfRawTracepointOpenCmd command
	BpfRawTracepointOpenCmd
	// BpfBtfLoadCmd command
	BpfBtfLoadCmd
	// BpfBtfGetFdByIDCmd command
	BpfBtfGetFdByIDCmd
	// BpfTaskFdQueryCmd command
	BpfTaskFdQueryCmd
	// BpfMapLookupAndDeleteElemCmd command
	BpfMapLookupAndDeleteElemCmd
	// BpfMapFreezeCmd command
	BpfMapFreezeCmd
	// BpfBtfGetNextIDCmd command
	BpfBtfGetNextIDCmd
	// BpfMapLookupBatchCmd command
	BpfMapLookupBatchCmd
	// BpfMapLookupAndDeleteBatchCmd command
	BpfMapLookupAndDeleteBatchCmd
	// BpfMapUpdateBatchCmd command
	BpfMapUpdateBatchCmd
	// BpfMapDeleteBatchCmd command
	BpfMapDeleteBatchCmd
	// BpfLinkCreateCmd command
	BpfLinkCreateCmd
	// BpfLinkUpdateCmd command
	BpfLinkUpdateCmd
	// BpfLinkGetFdByIDCmd command
	BpfLinkGetFdByIDCmd
	// BpfLinkGetNextIDCmd command
	BpfLinkGetNextIDCmd
	// BpfEnableStatsCmd command
	BpfEnableStatsCmd
	// BpfIterCreateCmd command
	BpfIterCreateCmd
	// BpfLinkDetachCmd command
	BpfLinkDetachCmd
	// BpfProgBindMapCmd command
	BpfProgBindMapCmd
)

// BPFHelperFunc represents a BPF helper function
type BPFHelperFunc uint32

func (f BPFHelperFunc) String() string {
	return bpfHelperFuncStrings[uint32(f)]
}

// StringifyHelpersList returns a string list representation of a list of helpers
func StringifyHelpersList(input []uint32) []string {
	helpers := make([]string, len(input))
	for i, helper := range input {
		helpers[i] = BPFHelperFunc(helper).String()
	}
	return helpers
}

const (
	// BpfUnspec helper function
	BpfUnspec BPFHelperFunc = iota
	// BpfMapLookupElem helper function
	BpfMapLookupElem
	// BpfMapUpdateElem helper function
	BpfMapUpdateElem
	// BpfMapDeleteElem helper function
	BpfMapDeleteElem
	// BpfProbeRead helper function
	BpfProbeRead
	// BpfKtimeGetNs helper function
	BpfKtimeGetNs
	// BpfTracePrintk helper function
	BpfTracePrintk
	// BpfGetPrandomU32 helper function
	BpfGetPrandomU32
	// BpfGetSmpProcessorID helper function
	BpfGetSmpProcessorID
	// BpfSkbStoreBytes helper function
	BpfSkbStoreBytes
	// BpfL3CsumReplace helper function
	BpfL3CsumReplace
	// BpfL4CsumReplace helper function
	BpfL4CsumReplace
	// BpfTailCall helper function
	BpfTailCall
	// BpfCloneRedirect helper function
	BpfCloneRedirect
	// BpfGetCurrentPidTgid helper function
	BpfGetCurrentPidTgid
	// BpfGetCurrentUIDGid helper function
	BpfGetCurrentUIDGid
	// BpfGetCurrentComm helper function
	BpfGetCurrentComm
	// BpfGetCgroupClassid helper function
	BpfGetCgroupClassid
	// BpfSkbVlanPush helper function
	BpfSkbVlanPush
	// BpfSkbVlanPop helper function
	BpfSkbVlanPop
	// BpfSkbGetTunnelKey helper function
	BpfSkbGetTunnelKey
	// BpfSkbSetTunnelKey helper function
	BpfSkbSetTunnelKey
	// BpfPerfEventRead helper function
	BpfPerfEventRead
	// BpfRedirect helper function
	BpfRedirect
	// BpfGetRouteRealm helper function
	BpfGetRouteRealm
	// BpfPerfEventOutput helper function
	BpfPerfEventOutput
	// BpfSkbLoadBytes helper function
	BpfSkbLoadBytes
	// BpfGetStackid helper function
	BpfGetStackid
	// BpfCsumDiff helper function
	BpfCsumDiff
	// BpfSkbGetTunnelOpt helper function
	BpfSkbGetTunnelOpt
	// BpfSkbSetTunnelOpt helper function
	BpfSkbSetTunnelOpt
	// BpfSkbChangeProto helper function
	BpfSkbChangeProto
	// BpfSkbChangeType helper function
	BpfSkbChangeType
	// BpfSkbUnderCgroup helper function
	BpfSkbUnderCgroup
	// BpfGetHashRecalc helper function
	BpfGetHashRecalc
	// BpfGetCurrentTask helper function
	BpfGetCurrentTask
	// BpfProbeWriteUser helper function
	BpfProbeWriteUser
	// BpfCurrentTaskUnderCgroup helper function
	BpfCurrentTaskUnderCgroup
	// BpfSkbChangeTail helper function
	BpfSkbChangeTail
	// BpfSkbPullData helper function
	BpfSkbPullData
	// BpfCsumUpdate helper function
	BpfCsumUpdate
	// BpfSetHashInvalid helper function
	BpfSetHashInvalid
	// BpfGetNumaNodeID helper function
	BpfGetNumaNodeID
	// BpfSkbChangeHead helper function
	BpfSkbChangeHead
	// BpfXdpAdjustHead helper function
	BpfXdpAdjustHead
	// BpfProbeReadStr helper function
	BpfProbeReadStr
	// BpfGetSocketCookie helper function
	BpfGetSocketCookie
	// BpfGetSocketUID helper function
	BpfGetSocketUID
	// BpfSetHash helper function
	BpfSetHash
	// BpfSetsockopt helper function
	BpfSetsockopt
	// BpfSkbAdjustRoom helper function
	BpfSkbAdjustRoom
	// BpfRedirectMap helper function
	BpfRedirectMap
	// BpfSkRedirectMap helper function
	BpfSkRedirectMap
	// BpfSockMapUpdate helper function
	BpfSockMapUpdate
	// BpfXdpAdjustMeta helper function
	BpfXdpAdjustMeta
	// BpfPerfEventReadValue helper function
	BpfPerfEventReadValue
	// BpfPerfProgReadValue helper function
	BpfPerfProgReadValue
	// BpfGetsockopt helper function
	BpfGetsockopt
	// BpfOverrideReturn helper function
	BpfOverrideReturn
	// BpfSockOpsCbFlagsSet helper function
	BpfSockOpsCbFlagsSet
	// BpfMsgRedirectMap helper function
	BpfMsgRedirectMap
	// BpfMsgApplyBytes helper function
	BpfMsgApplyBytes
	// BpfMsgCorkBytes helper function
	BpfMsgCorkBytes
	// BpfMsgPullData helper function
	BpfMsgPullData
	// BpfBind helper function
	BpfBind
	// BpfXdpAdjustTail helper function
	BpfXdpAdjustTail
	// BpfSkbGetXfrmState helper function
	BpfSkbGetXfrmState
	// BpfGetStack helper function
	BpfGetStack
	// BpfSkbLoadBytesRelative helper function
	BpfSkbLoadBytesRelative
	// BpfFibLookup helper function
	BpfFibLookup
	// BpfSockHashUpdate helper function
	BpfSockHashUpdate
	// BpfMsgRedirectHash helper function
	BpfMsgRedirectHash
	// BpfSkRedirectHash helper function
	BpfSkRedirectHash
	// BpfLwtPushEncap helper function
	BpfLwtPushEncap
	// BpfLwtSeg6StoreBytes helper function
	BpfLwtSeg6StoreBytes
	// BpfLwtSeg6AdjustSrh helper function
	BpfLwtSeg6AdjustSrh
	// BpfLwtSeg6Action helper function
	BpfLwtSeg6Action
	// BpfRcRepeat helper function
	BpfRcRepeat
	// BpfRcKeydown helper function
	BpfRcKeydown
	// BpfSkbCgroupID helper function
	BpfSkbCgroupID
	// BpfGetCurrentCgroupID helper function
	BpfGetCurrentCgroupID
	// BpfGetLocalStorage helper function
	BpfGetLocalStorage
	// BpfSkSelectReuseport helper function
	BpfSkSelectReuseport
	// BpfSkbAncestorCgroupID helper function
	BpfSkbAncestorCgroupID
	// BpfSkLookupTCP helper function
	BpfSkLookupTCP
	// BpfSkLookupUDP helper function
	BpfSkLookupUDP
	// BpfSkRelease helper function
	BpfSkRelease
	// BpfMapPushElem helper function
	BpfMapPushElem
	// BpfMapPopElem helper function
	BpfMapPopElem
	// BpfMapPeekElem helper function
	BpfMapPeekElem
	// BpfMsgPushData helper function
	BpfMsgPushData
	// BpfMsgPopData helper function
	BpfMsgPopData
	// BpfRcPointerRel helper function
	BpfRcPointerRel
	// BpfSpinLock helper function
	BpfSpinLock
	// BpfSpinUnlock helper function
	BpfSpinUnlock
	// BpfSkFullsock helper function
	BpfSkFullsock
	// BpfTCPSock helper function
	BpfTCPSock
	// BpfSkbEcnSetCe helper function
	BpfSkbEcnSetCe
	// BpfGetListenerSock helper function
	BpfGetListenerSock
	// BpfSkcLookupTCP helper function
	BpfSkcLookupTCP
	// BpfTCPCheckSyncookie helper function
	BpfTCPCheckSyncookie
	// BpfSysctlGetName helper function
	BpfSysctlGetName
	// BpfSysctlGetCurrentValue helper function
	BpfSysctlGetCurrentValue
	// BpfSysctlGetNewValue helper function
	BpfSysctlGetNewValue
	// BpfSysctlSetNewValue helper function
	BpfSysctlSetNewValue
	// BpfStrtol helper function
	BpfStrtol
	// BpfStrtoul helper function
	BpfStrtoul
	// BpfSkStorageGet helper function
	BpfSkStorageGet
	// BpfSkStorageDelete helper function
	BpfSkStorageDelete
	// BpfSendSignal helper function
	BpfSendSignal
	// BpfTCPGenSyncookie helper function
	BpfTCPGenSyncookie
	// BpfSkbOutput helper function
	BpfSkbOutput
	// BpfProbeReadUser helper function
	BpfProbeReadUser
	// BpfProbeReadKernel helper function
	BpfProbeReadKernel
	// BpfProbeReadUserStr helper function
	BpfProbeReadUserStr
	// BpfProbeReadKernelStr helper function
	BpfProbeReadKernelStr
	// BpfTCPSendAck helper function
	BpfTCPSendAck
	// BpfSendSignalThread helper function
	BpfSendSignalThread
	// BpfJiffies64 helper function
	BpfJiffies64
	// BpfReadBranchRecords helper function
	BpfReadBranchRecords
	// BpfGetNsCurrentPidTgid helper function
	BpfGetNsCurrentPidTgid
	// BpfXdpOutput helper function
	BpfXdpOutput
	// BpfGetNetnsCookie helper function
	BpfGetNetnsCookie
	// BpfGetCurrentAncestorCgroupID helper function
	BpfGetCurrentAncestorCgroupID
	// BpfSkAssign helper function
	BpfSkAssign
	// BpfKtimeGetBootNs helper function
	BpfKtimeGetBootNs
	// BpfSeqPrintf helper function
	BpfSeqPrintf
	// BpfSeqWrite helper function
	BpfSeqWrite
	// BpfSkCgroupID helper function
	BpfSkCgroupID
	// BpfSkAncestorCgroupID helper function
	BpfSkAncestorCgroupID
	// BpfRingbufOutput helper function
	BpfRingbufOutput
	// BpfRingbufReserve helper function
	BpfRingbufReserve
	// BpfRingbufSubmit helper function
	BpfRingbufSubmit
	// BpfRingbufDiscard helper function
	BpfRingbufDiscard
	// BpfRingbufQuery helper function
	BpfRingbufQuery
	// BpfCsumLevel helper function
	BpfCsumLevel
	// BpfSkcToTCP6Sock helper function
	BpfSkcToTCP6Sock
	// BpfSkcToTCPSock helper function
	BpfSkcToTCPSock
	// BpfSkcToTCPTimewaitSock helper function
	BpfSkcToTCPTimewaitSock
	// BpfSkcToTCPRequestSock helper function
	BpfSkcToTCPRequestSock
	// BpfSkcToUDP6Sock helper function
	BpfSkcToUDP6Sock
	// BpfGetTaskStack helper function
	BpfGetTaskStack
	// BpfLoadHdrOpt helper function
	BpfLoadHdrOpt
	// BpfStoreHdrOpt helper function
	BpfStoreHdrOpt
	// BpfReserveHdrOpt helper function
	BpfReserveHdrOpt
	// BpfInodeStorageGet helper function
	BpfInodeStorageGet
	// BpfInodeStorageDelete helper function
	BpfInodeStorageDelete
	// BpfDPath helper function
	BpfDPath
	// BpfCopyFromUser helper function
	BpfCopyFromUser
	// BpfSnprintfBtf helper function
	BpfSnprintfBtf
	// BpfSeqPrintfBtf helper function
	BpfSeqPrintfBtf
	// BpfSkbCgroupClassid helper function
	BpfSkbCgroupClassid
	// BpfRedirectNeigh helper function
	BpfRedirectNeigh
	// BpfPerCPUPtr helper function
	BpfPerCPUPtr
	// BpfThisCPUPtr helper function
	BpfThisCPUPtr
	// BpfRedirectPeer helper function
	BpfRedirectPeer
	// BpfTaskStorageGet helper function
	BpfTaskStorageGet
	// BpfTaskStorageDelete helper function
	BpfTaskStorageDelete
	// BpfGetCurrentTaskBtf helper function
	BpfGetCurrentTaskBtf
	// BpfBprmOptsSet helper function
	BpfBprmOptsSet
	// BpfKtimeGetCoarseNs helper function
	BpfKtimeGetCoarseNs
	// BpfImaInodeHash helper function
	BpfImaInodeHash
	// BpfSockFromFile helper function
	BpfSockFromFile
	// BpfCheckMtu helper function
	BpfCheckMtu
	// BpfForEachMapElem helper function
	BpfForEachMapElem
	// BpfSnprintf helper function
	BpfSnprintf
)

// BPFMapType is used to define map type constants
type BPFMapType uint32

func (t BPFMapType) String() string {
	return bpfMapTypeStrings[uint32(t)]
}

const (
	// BpfMapTypeUnspec map type
	BpfMapTypeUnspec BPFMapType = iota
	// BpfMapTypeHash map type
	BpfMapTypeHash
	// BpfMapTypeArray map type
	BpfMapTypeArray
	// BpfMapTypeProgArray map type
	BpfMapTypeProgArray
	// BpfMapTypePerfEventArray map type
	BpfMapTypePerfEventArray
	// BpfMapTypePercpuHash map type
	BpfMapTypePercpuHash
	// BpfMapTypePercpuArray map type
	BpfMapTypePercpuArray
	// BpfMapTypeStackTrace map type
	BpfMapTypeStackTrace
	// BpfMapTypeCgroupArray map type
	BpfMapTypeCgroupArray
	// BpfMapTypeLruHash map type
	BpfMapTypeLruHash
	// BpfMapTypeLruPercpuHash map type
	BpfMapTypeLruPercpuHash
	// BpfMapTypeLpmTrie map type
	BpfMapTypeLpmTrie
	// BpfMapTypeArrayOfMaps map type
	BpfMapTypeArrayOfMaps
	// BpfMapTypeHashOfMaps map type
	BpfMapTypeHashOfMaps
	// BpfMapTypeDevmap map type
	BpfMapTypeDevmap
	// BpfMapTypeSockmap map type
	BpfMapTypeSockmap
	// BpfMapTypeCPUmap map type
	BpfMapTypeCPUmap
	// BpfMapTypeXskmap map type
	BpfMapTypeXskmap
	// BpfMapTypeSockhash map type
	BpfMapTypeSockhash
	// BpfMapTypeCgroupStorage map type
	BpfMapTypeCgroupStorage
	// BpfMapTypeReuseportSockarray map type
	BpfMapTypeReuseportSockarray
	// BpfMapTypePercpuCgroupStorage map type
	BpfMapTypePercpuCgroupStorage
	// BpfMapTypeQueue map type
	BpfMapTypeQueue
	// BpfMapTypeStack map type
	BpfMapTypeStack
	// BpfMapTypeSkStorage map type
	BpfMapTypeSkStorage
	// BpfMapTypeDevmapHash map type
	BpfMapTypeDevmapHash
	// BpfMapTypeStructOps map type
	BpfMapTypeStructOps
	// BpfMapTypeRingbuf map type
	BpfMapTypeRingbuf
	// BpfMapTypeInodeStorage map type
	BpfMapTypeInodeStorage
	// BpfMapTypeTaskStorage map type
	BpfMapTypeTaskStorage
)

// BPFProgramType is used to define program type constants
type BPFProgramType uint32

func (t BPFProgramType) String() string {
	return bpfProgramTypeStrings[uint32(t)]
}

const (
	// BpfProgTypeUnspec program type
	BpfProgTypeUnspec BPFProgramType = iota
	// BpfProgTypeSocketFilter program type
	BpfProgTypeSocketFilter
	// BpfProgTypeKprobe program type
	BpfProgTypeKprobe
	// BpfProgTypeSchedCls program type
	BpfProgTypeSchedCls
	// BpfProgTypeSchedAct program type
	BpfProgTypeSchedAct
	// BpfProgTypeTracepoint program type
	BpfProgTypeTracepoint
	// BpfProgTypeXdp program type
	BpfProgTypeXdp
	// BpfProgTypePerfEvent program type
	BpfProgTypePerfEvent
	// BpfProgTypeCgroupSkb program type
	BpfProgTypeCgroupSkb
	// BpfProgTypeCgroupSock program type
	BpfProgTypeCgroupSock
	// BpfProgTypeLwtIn program type
	BpfProgTypeLwtIn
	// BpfProgTypeLwtOut program type
	BpfProgTypeLwtOut
	// BpfProgTypeLwtXmit program type
	BpfProgTypeLwtXmit
	// BpfProgTypeSockOps program type
	BpfProgTypeSockOps
	// BpfProgTypeSkSkb program type
	BpfProgTypeSkSkb
	// BpfProgTypeCgroupDevice program type
	BpfProgTypeCgroupDevice
	// BpfProgTypeSkMsg program type
	BpfProgTypeSkMsg
	// BpfProgTypeRawTracepoint program type
	BpfProgTypeRawTracepoint
	// BpfProgTypeCgroupSockAddr program type
	BpfProgTypeCgroupSockAddr
	// BpfProgTypeLwtSeg6local program type
	BpfProgTypeLwtSeg6local
	// BpfProgTypeLircMode2 program type
	BpfProgTypeLircMode2
	// BpfProgTypeSkReuseport program type
	BpfProgTypeSkReuseport
	// BpfProgTypeFlowDissector program type
	BpfProgTypeFlowDissector
	// BpfProgTypeCgroupSysctl program type
	BpfProgTypeCgroupSysctl
	// BpfProgTypeRawTracepointWritable program type
	BpfProgTypeRawTracepointWritable
	// BpfProgTypeCgroupSockopt program type
	BpfProgTypeCgroupSockopt
	// BpfProgTypeTracing program type
	BpfProgTypeTracing
	// BpfProgTypeStructOps program type
	BpfProgTypeStructOps
	// BpfProgTypeExt program type
	BpfProgTypeExt
	// BpfProgTypeLsm program type
	BpfProgTypeLsm
	// BpfProgTypeSkLookup program type
	BpfProgTypeSkLookup
)

// BPFAttachType is used to define attach type constants
type BPFAttachType uint32

func (t BPFAttachType) String() string {
	return bpfAttachTypeStrings[uint32(t)]
}

const (
	// BpfCgroupInetIngress attach type
	BpfCgroupInetIngress BPFAttachType = iota + 1
	// BpfCgroupInetEgress attach type
	BpfCgroupInetEgress
	// BpfCgroupInetSockCreate attach type
	BpfCgroupInetSockCreate
	// BpfCgroupSockOps attach type
	BpfCgroupSockOps
	// BpfSkSkbStreamParser attach type
	BpfSkSkbStreamParser
	// BpfSkSkbStreamVerdict attach type
	BpfSkSkbStreamVerdict
	// BpfCgroupDevice attach type
	BpfCgroupDevice
	// BpfSkMsgVerdict attach type
	BpfSkMsgVerdict
	// BpfCgroupInet4Bind attach type
	BpfCgroupInet4Bind
	// BpfCgroupInet6Bind attach type
	BpfCgroupInet6Bind
	// BpfCgroupInet4Connect attach type
	BpfCgroupInet4Connect
	// BpfCgroupInet6Connect attach type
	BpfCgroupInet6Connect
	// BpfCgroupInet4PostBind attach type
	BpfCgroupInet4PostBind
	// BpfCgroupInet6PostBind attach type
	BpfCgroupInet6PostBind
	// BpfCgroupUDP4Sendmsg attach type
	BpfCgroupUDP4Sendmsg
	// BpfCgroupUDP6Sendmsg attach type
	BpfCgroupUDP6Sendmsg
	// BpfLircMode2 attach type
	BpfLircMode2
	// BpfFlowDissector attach type
	BpfFlowDissector
	// BpfCgroupSysctl attach type
	BpfCgroupSysctl
	// BpfCgroupUDP4Recvmsg attach type
	BpfCgroupUDP4Recvmsg
	// BpfCgroupUDP6Recvmsg attach type
	BpfCgroupUDP6Recvmsg
	// BpfCgroupGetsockopt attach type
	BpfCgroupGetsockopt
	// BpfCgroupSetsockopt attach type
	BpfCgroupSetsockopt
	// BpfTraceRawTp attach type
	BpfTraceRawTp
	// BpfTraceFentry attach type
	BpfTraceFentry
	// BpfTraceFexit attach type
	BpfTraceFexit
	// BpfModifyReturn attach type
	BpfModifyReturn
	// BpfLsmMac attach type
	BpfLsmMac
	// BpfTraceIter attach type
	BpfTraceIter
	// BpfCgroupInet4Getpeername attach type
	BpfCgroupInet4Getpeername
	// BpfCgroupInet6Getpeername attach type
	BpfCgroupInet6Getpeername
	// BpfCgroupInet4Getsockname attach type
	BpfCgroupInet4Getsockname
	// BpfCgroupInet6Getsockname attach type
	BpfCgroupInet6Getsockname
	// BpfXdpDevmap attach type
	BpfXdpDevmap
	// BpfCgroupInetSockRelease attach type
	BpfCgroupInetSockRelease
	// BpfXdpCPUmap attach type
	BpfXdpCPUmap
	// BpfSkLookup attach type
	BpfSkLookup
	// BpfXdp attach type
	BpfXdp
	// BpfSkSkbVerdict attach type
	BpfSkSkbVerdict
)

// PTraceRequest represents a ptrace request value
type PTraceRequest uint32

func (f PTraceRequest) String() string {
	for val, str := range ptraceFlagsStrings {
		if val == uint32(f) {
			return str
		}
	}
	return fmt.Sprintf("%d", f)
}

// VMFlag represents a VM_* bitmask value
type VMFlag int

func (vmf VMFlag) String() string {
	return bitmaskToString(int(vmf), vmStrings)
}

// Protection represents a virtual memory protection bitmask value
type Protection int

func (p Protection) String() string {
	return bitmaskToString(int(p), protStrings)
}

// MMapFlag represents a mmap flag value
type MMapFlag int

func (mmf MMapFlag) String() string {
	return bitmaskToString(int(mmf), mmapFlagStrings)
}

// Signal represents a type of unix signal (ie, SIGKILL, SIGSTOP etc)
type Signal int

func (sig Signal) String() string {
	return signalStrings[int(sig)]
}

// PipeBufFlag represents a pipe buffer flag
type PipeBufFlag int

func (pbf PipeBufFlag) String() string {
	return bitmaskToString(int(pbf), pipeBufFlagStrings)
}

// AddressFamily represents a family address (AF_INET, AF_INET6, AF_UNIX etc)
type AddressFamily int

func (af AddressFamily) String() string {
	return addressFamilyStrings[uint16(af)]
}

const (
	// PipeBufFlagLRU pipe buffer flag
	PipeBufFlagLRU PipeBufFlag = 0x1 /* page is on the LRU */
	// PipeBufFlagAtomic pipe buffer flag
	PipeBufFlagAtomic PipeBufFlag = 0x2 /* was atomically mapped */
	// PipeBufFlagGift pipe buffer flag
	PipeBufFlagGift PipeBufFlag = 0x4 /* page is a gift */
	// PipeBufFlagPacket pipe buffer flag
	PipeBufFlagPacket PipeBufFlag = 0x8 /* read() as a packet */
	// PipeBufFlagCanMerge pipe buffer flag
	PipeBufFlagCanMerge PipeBufFlag = 0x10 /* can merge buffers */
	// PipeBufFlagWhole pipe buffer flag
	PipeBufFlagWhole PipeBufFlag = 0x20 /* read() must return entire buffer or error */
	// PipeBufFlagLoss pipe buffer flag
	PipeBufFlagLoss PipeBufFlag = 0x40 /* Message loss happened after this buffer */
)

// QClass is used to declare the qclass field of a DNS request
type QClass uint32

func (qc QClass) String() string {
	if val, ok := dnsQClassStrings[uint32(qc)]; ok {
		return val
	}
	return fmt.Sprintf("qclass(%d)", qc)
}

// QType is used to declare the qtype field of a DNS request
type QType uint32

func (qt QType) String() string {
	if val, ok := dnsQTypeStrings[uint32(qt)]; ok {
		return val
	}
	return fmt.Sprintf("qtype(%d)", qt)
}

// L3Protocol Network protocols
type L3Protocol uint16

func (proto L3Protocol) String() string {
	return l3ProtocolStrings[proto]
}

const (
	// EthPLOOP Ethernet Loopback packet
	EthPLOOP L3Protocol = 0x0060
	// EthPPUP Xerox PUP packet
	EthPPUP L3Protocol = 0x0200
	// EthPPUPAT Xerox PUP Addr Trans packet
	EthPPUPAT L3Protocol = 0x0201
	// EthPTSN TSN (IEEE 1722) packet
	EthPTSN L3Protocol = 0x22F0
	// EthPIP Internet Protocol packet
	EthPIP L3Protocol = 0x0800
	// EthPX25 CCITT X.25
	EthPX25 L3Protocol = 0x0805
	// EthPARP Address Resolution packet
	EthPARP L3Protocol = 0x0806
	// EthPBPQ G8BPQ AX.25 Ethernet Packet    [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPBPQ L3Protocol = 0x08FF
	// EthPIEEEPUP Xerox IEEE802.3 PUP packet
	EthPIEEEPUP L3Protocol = 0x0a00
	// EthPIEEEPUPAT Xerox IEEE802.3 PUP Addr Trans packet
	EthPIEEEPUPAT L3Protocol = 0x0a01
	// EthPBATMAN B.A.T.M.A.N.-Advanced packet [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPBATMAN L3Protocol = 0x4305
	// EthPDEC DEC Assigned proto
	EthPDEC L3Protocol = 0x6000
	// EthPDNADL DEC DNA Dump/Load
	EthPDNADL L3Protocol = 0x6001
	// EthPDNARC DEC DNA Remote Console
	EthPDNARC L3Protocol = 0x6002
	// EthPDNART DEC DNA Routing
	EthPDNART L3Protocol = 0x6003
	// EthPLAT DEC LAT
	EthPLAT L3Protocol = 0x6004
	// EthPDIAG DEC Diagnostics
	EthPDIAG L3Protocol = 0x6005
	// EthPCUST DEC Customer use
	EthPCUST L3Protocol = 0x6006
	// EthPSCA DEC Systems Comms Arch
	EthPSCA L3Protocol = 0x6007
	// EthPTEB Trans Ether Bridging
	EthPTEB L3Protocol = 0x6558
	// EthPRARP Reverse Addr Res packet
	EthPRARP L3Protocol = 0x8035
	// EthPATALK Appletalk DDP
	EthPATALK L3Protocol = 0x809B
	// EthPAARP Appletalk AARP
	EthPAARP L3Protocol = 0x80F3
	// EthP8021Q 802.1Q VLAN Extended Header
	EthP8021Q L3Protocol = 0x8100
	// EthPERSPAN ERSPAN type II
	EthPERSPAN L3Protocol = 0x88BE
	// EthPIPX IPX over DIX
	EthPIPX L3Protocol = 0x8137
	// EthPIPV6 IPv6 over bluebook
	EthPIPV6 L3Protocol = 0x86DD
	// EthPPAUSE IEEE Pause frames. See 802.3 31B
	EthPPAUSE L3Protocol = 0x8808
	// EthPSLOW Slow Protocol. See 802.3ad 43B
	EthPSLOW L3Protocol = 0x8809
	// EthPWCCP Web-cache coordination protocol defined in draft-wilson-wrec-wccp-v2-00.txt
	EthPWCCP L3Protocol = 0x883E
	// EthPMPLSUC MPLS Unicast traffic
	EthPMPLSUC L3Protocol = 0x8847
	// EthPMPLSMC MPLS Multicast traffic
	EthPMPLSMC L3Protocol = 0x8848
	// EthPATMMPOA MultiProtocol Over ATM
	EthPATMMPOA L3Protocol = 0x884c
	// EthPPPPDISC PPPoE discovery messages
	EthPPPPDISC L3Protocol = 0x8863
	// EthPPPPSES PPPoE session messages
	EthPPPPSES L3Protocol = 0x8864
	// EthPLinkCTL HPNA, wlan link local tunnel
	EthPLinkCTL L3Protocol = 0x886c
	// EthPATMFATE Frame-based ATM Transport over Ethernet
	EthPATMFATE L3Protocol = 0x8884
	// EthPPAE Port Access Entity (IEEE 802.1X)
	EthPPAE L3Protocol = 0x888E
	// EthPAOE ATA over Ethernet
	EthPAOE L3Protocol = 0x88A2
	// EthP8021AD 802.1ad Service VLAN
	EthP8021AD L3Protocol = 0x88A8
	// EthP802EX1 802.1 Local Experimental 1.
	EthP802EX1 L3Protocol = 0x88B5
	// EthPTIPC TIPC
	EthPTIPC L3Protocol = 0x88CA
	// EthPMACSEC 802.1ae MACsec
	EthPMACSEC L3Protocol = 0x88E5
	// EthP8021AH 802.1ah Backbone Service Tag
	EthP8021AH L3Protocol = 0x88E7
	// EthPMVRP 802.1Q MVRP
	EthPMVRP L3Protocol = 0x88F5
	// EthP1588 IEEE 1588 Timesync
	EthP1588 L3Protocol = 0x88F7
	// EthPNCSI NCSI protocol
	EthPNCSI L3Protocol = 0x88F8
	// EthPPRP IEC 62439-3 PRP/HSRv0
	EthPPRP L3Protocol = 0x88FB
	// EthPFCOE Fibre Channel over Ethernet
	EthPFCOE L3Protocol = 0x8906
	// EthPIBOE Infiniband over Ethernet
	EthPIBOE L3Protocol = 0x8915
	// EthPTDLS TDLS
	EthPTDLS L3Protocol = 0x890D
	// EthPFIP FCoE Initialization Protocol
	EthPFIP L3Protocol = 0x8914
	// EthP80221 IEEE 802.21 Media Independent Handover Protocol
	EthP80221 L3Protocol = 0x8917
	// EthPHSR IEC 62439-3 HSRv1
	EthPHSR L3Protocol = 0x892F
	// EthPNSH Network Service Header
	EthPNSH L3Protocol = 0x894F
	// EthPLOOPBACK Ethernet loopback packet, per IEEE 802.3
	EthPLOOPBACK L3Protocol = 0x9000
	// EthPQINQ1 deprecated QinQ VLAN [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPQINQ1 L3Protocol = 0x9100
	// EthPQINQ2 deprecated QinQ VLAN [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPQINQ2 L3Protocol = 0x9200
	// EthPQINQ3 deprecated QinQ VLAN [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPQINQ3 L3Protocol = 0x9300
	// EthPEDSA Ethertype DSA [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPEDSA L3Protocol = 0xDADA
	// EthPIFE ForCES inter-FE LFB type
	EthPIFE L3Protocol = 0xED3E
	// EthPAFIUCV IBM afiucv [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPAFIUCV L3Protocol = 0xFBFB
	// EthP8023MIN If the value in the ethernet type is less than this value then the frame is Ethernet II. Else it is 802.3
	EthP8023MIN L3Protocol = 0x0600
	// EthPIPV6HopByHop IPv6 Hop by hop option
	EthPIPV6HopByHop L3Protocol = 0x000
	// EthP8023 Dummy type for 802.3 frames
	EthP8023 L3Protocol = 0x0001
	// EthPAX25 Dummy protocol id for AX.25
	EthPAX25 L3Protocol = 0x0002
	// EthPALL Every packet (be careful!!!)
	EthPALL L3Protocol = 0x0003
	// EthP8022 802.2 frames
	EthP8022 L3Protocol = 0x0004
	// EthPSNAP Internal only
	EthPSNAP L3Protocol = 0x0005
	// EthPDDCMP DEC DDCMP: Internal only
	EthPDDCMP L3Protocol = 0x0006
	// EthPWANPPP Dummy type for WAN PPP frames*/
	EthPWANPPP L3Protocol = 0x0007
	// EthPPPPMP Dummy type for PPP MP frames
	EthPPPPMP L3Protocol = 0x0008
	// EthPLOCALTALK Localtalk pseudo type
	EthPLOCALTALK L3Protocol = 0x0009
	// EthPCAN CAN: Controller Area Network
	EthPCAN L3Protocol = 0x000C
	// EthPCANFD CANFD: CAN flexible data rate*/
	EthPCANFD L3Protocol = 0x000D
	// EthPPPPTALK Dummy type for Atalk over PPP*/
	EthPPPPTALK L3Protocol = 0x0010
	// EthPTR8022 802.2 frames
	EthPTR8022 L3Protocol = 0x0011
	// EthPMOBITEX Mobitex (kaz@cafe.net)
	EthPMOBITEX L3Protocol = 0x0015
	// EthPCONTROL Card specific control frames
	EthPCONTROL L3Protocol = 0x0016
	// EthPIRDA Linux-IrDA
	EthPIRDA L3Protocol = 0x0017
	// EthPECONET Acorn Econet
	EthPECONET L3Protocol = 0x0018
	// EthPHDLC HDLC frames
	EthPHDLC L3Protocol = 0x0019
	// EthPARCNET 1A for ArcNet :-)
	EthPARCNET L3Protocol = 0x001A
	// EthPDSA Distributed Switch Arch.
	EthPDSA L3Protocol = 0x001B
	// EthPTRAILER Trailer switch tagging
	EthPTRAILER L3Protocol = 0x001C
	// EthPPHONET Nokia Phonet frames
	EthPPHONET L3Protocol = 0x00F5
	// EthPIEEE802154 IEEE802.15.4 frame
	EthPIEEE802154 L3Protocol = 0x00F6
	// EthPCAIF ST-Ericsson CAIF protocol
	EthPCAIF L3Protocol = 0x00F7
	// EthPXDSA Multiplexed DSA protocol
	EthPXDSA L3Protocol = 0x00F8
	// EthPMAP Qualcomm multiplexing and aggregation protocol
	EthPMAP L3Protocol = 0x00F9
)

// L4Protocol transport protocols
type L4Protocol uint16

func (proto L4Protocol) String() string {
	return l4ProtocolStrings[proto]
}

const (
	// IPProtoIP Dummy protocol for TCP
	IPProtoIP L4Protocol = 0
	// IPProtoICMP Internet Control Message Protocol (IPv4)
	IPProtoICMP L4Protocol = 1
	// IPProtoIGMP Internet Group Management Protocol
	IPProtoIGMP L4Protocol = 2
	// IPProtoIPIP IPIP tunnels (older KA9Q tunnels use 94)
	IPProtoIPIP L4Protocol = 4
	// IPProtoTCP Transmission Control Protocol
	IPProtoTCP L4Protocol = 6
	// IPProtoEGP Exterior Gateway Protocol
	IPProtoEGP L4Protocol = 8
	// IPProtoIGP Interior Gateway Protocol (any private interior gateway (used by Cisco for their IGRP))
	IPProtoIGP L4Protocol = 9
	// IPProtoPUP PUP protocol
	IPProtoPUP L4Protocol = 12
	// IPProtoUDP User Datagram Protocol
	IPProtoUDP L4Protocol = 17
	// IPProtoIDP XNS IDP protocol
	IPProtoIDP L4Protocol = 22
	// IPProtoTP SO Transport Protocol Class 4
	IPProtoTP L4Protocol = 29
	// IPProtoDCCP Datagram Congestion Control Protocol
	IPProtoDCCP L4Protocol = 33
	// IPProtoIPV6 IPv6-in-IPv4 tunnelling
	IPProtoIPV6 L4Protocol = 41
	// IPProtoRSVP RSVP Protocol
	IPProtoRSVP L4Protocol = 46
	// IPProtoGRE Cisco GRE tunnels (rfc 1701,1702)
	IPProtoGRE L4Protocol = 47
	// IPProtoESP Encapsulation Security Payload protocol
	IPProtoESP L4Protocol = 50
	// IPProtoAH Authentication Header protocol
	IPProtoAH L4Protocol = 51
	// IPProtoICMPV6 Internet Control Message Protocol (IPv6)
	IPProtoICMPV6 L4Protocol = 58
	// IPProtoMTP Multicast Transport Protocol
	IPProtoMTP L4Protocol = 92
	// IPProtoBEETPH IP option pseudo header for BEET
	IPProtoBEETPH L4Protocol = 94
	// IPProtoENCAP Encapsulation Header
	IPProtoENCAP L4Protocol = 98
	// IPProtoPIM Protocol Independent Multicast
	IPProtoPIM L4Protocol = 103
	// IPProtoCOMP Compression Header Protocol
	IPProtoCOMP L4Protocol = 108
	// IPProtoSCTP Stream Control Transport Protocol
	IPProtoSCTP L4Protocol = 132
	// IPProtoUDPLITE UDP-Lite (RFC 3828)
	IPProtoUDPLITE L4Protocol = 136
	// IPProtoMPLS MPLS in IP (RFC 4023)
	IPProtoMPLS L4Protocol = 137
	// IPProtoRAW Raw IP packets
	IPProtoRAW L4Protocol = 255
)

// ExitCause represents the cause of a process termination
type ExitCause uint32

func (cause ExitCause) String() string {
	return exitCauseStrings[cause]
}

const (
	// ExitExited Process exited normally
	ExitExited ExitCause = iota
	// ExitCoreDumped Process was terminated with a coredump signal
	ExitCoreDumped
	// ExitSignaled Process was terminated with a signal other than a coredump
	ExitSignaled
)
