// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kprobe

import (
	"errors"
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	libebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	ssluprobes "github.com/DataDog/datadog-agent/pkg/network/tracer/connection/ssl-uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func enableProbe(enabled map[manager.ProbeIdentificationPair]struct{}, name probes.ProbeFuncName) {
	probeIdentifier := manager.ProbeIdentificationPair{
		EBPFFuncName: name,
		UID:          probeUID,
	}
	enabled[probeIdentifier] = struct{}{}
}

// enabledProbes returns a map of probes that are enabled per config settings.
// This map does not include the probes used exclusively in the offset guessing process.
func enabledProbes(c *config.Config, runtimeTracer, coreTracer bool) (map[manager.ProbeIdentificationPair]struct{}, error) {
	enabled := make(map[manager.ProbeIdentificationPair]struct{}, 0)

	kv410 := kernel.VersionCode(4, 1, 0)
	kv415 := kernel.VersionCode(4, 15, 0)
	kv470 := kernel.VersionCode(4, 7, 0)
	kv4180 := kernel.VersionCode(4, 18, 0)
	kv5180 := kernel.VersionCode(5, 18, 0)
	kv5190 := kernel.VersionCode(5, 19, 0)

	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, err
	}

	// Default to kprobe fallback for net_dev_queue (works on all kernel versions)
	netDevQueue := probes.DevQueueXmitNitKprobe

	// Upgrade to tracepoint or raw tracepoint based on kernel version and capabilities
	if features.HaveProgramType(libebpf.RawTracepoint) == nil {
		// Kernel >= 4.17 typically - use raw tracepoint (most efficient)
		netDevQueue = probes.NetDevQueueRawTracepoint
	} else if kv >= kv415 {
		// Kernel >= 4.15 - use regular tracepoint (multiple attachment supported)
		netDevQueue = probes.NetDevQueueTracepoint
	}
	// else: Kernel < 4.15 - keep kprobe fallback (no multiple tracepoint attachment)

	hasSendPage := util.HasTCPSendPage(kv)

	if c.CollectTCPv4Conns || c.CollectTCPv6Conns {
		if ClassificationSupported(c) {
			enableProbe(enabled, probes.ProtocolClassifierEntrySocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierTLSClientSocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierTLSServerSocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierQueuesSocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierDBsSocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierGRPCSocketFilter)
			enableProbe(enabled, netDevQueue)
			enableProbe(enabled, probes.TCPCloseCleanProtocolsReturn)
		}
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, probes.TCPSendMsg, probes.TCPSendMsgPre410, kv410))
		enableProbe(enabled, probes.TCPSendMsgReturn)
		if hasSendPage {
			enableProbe(enabled, probes.TCPSendPage)
			enableProbe(enabled, probes.TCPSendPageReturn)
		}
		// 5.19: remove noblock parameter in *_recvmsg https://github.com/torvalds/linux/commit/ec095263a965720e1ca39db1d9c5cd47846c789b
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, selectVersionBasedProbe(runtimeTracer, kv, probes.TCPRecvMsg, probes.TCPRecvMsgPre5190, kv5190), probes.TCPRecvMsgPre410, kv410))
		enableProbe(enabled, probes.TCPRecvMsgReturn)
		enableProbe(enabled, probes.TCPReadSock)
		enableProbe(enabled, probes.TCPReadSockReturn)
		enableProbe(enabled, probes.TCPClose)
		if c.CustomBatchingEnabled {
			enableProbe(enabled, probes.TCPCloseFlushReturn)
		}

		enableProbe(enabled, probes.TCPConnect)
		enableProbe(enabled, probes.TCPDone)
		if c.CustomBatchingEnabled {
			enableProbe(enabled, probes.TCPDoneFlushReturn)
		}
		enableProbe(enabled, probes.TCPFinishConnect)
		enableProbe(enabled, probes.InetCskAcceptReturn)
		enableProbe(enabled, probes.InetCskListenStop)
		// special case for tcp_retransmit_skb probe: on CO-RE,
		// we want to load the version that makes use of
		// the tcp_sock field, which is the same as the
		// runtime compiled implementation
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer || coreTracer, kv, probes.TCPRetransmit, probes.TCPRetransmitPre470, kv470))
		enableProbe(enabled, probes.TCPRetransmitRet)
		if runtimeTracer || coreTracer {
			// TCPEnterLoss and TCPEnterRecovery exist in both the runtime-compiled and
			// CO-RE kprobe ELFs. Not available on prebuilt.
			enableProbe(enabled, probes.TCPEnterLoss)
			enableProbe(enabled, probes.TCPEnterRecovery)
		}
	}

	if c.CollectUDPv4Conns {
		enableProbe(enabled, probes.UDPDestroySock)
		if c.CustomBatchingEnabled {
			enableProbe(enabled, probes.UDPDestroySockReturn)
		}
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, probes.IPMakeSkb, probes.IPMakeSkbPre4180, kv4180))
		enableProbe(enabled, probes.IPMakeSkbReturn)
		enableProbe(enabled, probes.InetBind)
		enableProbe(enabled, probes.InetBindRet)
		if hasSendPage {
			enableProbe(enabled, probes.UDPSendPage)
			enableProbe(enabled, probes.UDPSendPageReturn)
		}
		if kv >= kv5190 || runtimeTracer {
			enableProbe(enabled, probes.UDPRecvMsg)
		} else if kv >= kv470 {
			enableProbe(enabled, probes.UDPRecvMsgPre5190)
		} else if kv >= kv410 {
			enableProbe(enabled, probes.UDPRecvMsgPre470)
		} else {
			enableProbe(enabled, probes.UDPRecvMsgPre410)
		}
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer || coreTracer, kv, probes.UDPRecvMsgReturn, probes.UDPRecvMsgReturnPre470, kv470))
	}

	if c.CollectUDPv6Conns {
		enableProbe(enabled, probes.UDPv6DestroySock)
		if c.CustomBatchingEnabled {
			enableProbe(enabled, probes.UDPv6DestroySockReturn)
		}
		if kv >= kv5180 || runtimeTracer {
			// prebuilt shouldn't arrive here with 5.18+ and UDPv6 enabled
			if !coreTracer && !runtimeTracer {
				return nil, errors.New("UDPv6 does not function on prebuilt tracer with kernel versions 5.18+")
			}
			enableProbe(enabled, probes.IP6MakeSkb)
		} else if kv >= kv470 {
			enableProbe(enabled, probes.IP6MakeSkbPre5180)
		} else {
			enableProbe(enabled, probes.IP6MakeSkbPre470)
		}
		enableProbe(enabled, probes.IP6MakeSkbReturn)
		enableProbe(enabled, probes.Inet6Bind)
		enableProbe(enabled, probes.Inet6BindRet)
		if hasSendPage {
			enableProbe(enabled, probes.UDPSendPage)
			enableProbe(enabled, probes.UDPSendPageReturn)
		}
		if kv >= kv5190 || runtimeTracer {
			enableProbe(enabled, probes.UDPv6RecvMsg)
		} else if kv >= kv470 {
			enableProbe(enabled, probes.UDPv6RecvMsgPre5190)
		} else if kv >= kv410 {
			enableProbe(enabled, probes.UDPv6RecvMsgPre470)
		} else {
			enableProbe(enabled, probes.UDPv6RecvMsgPre410)
		}
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer || coreTracer, kv, probes.UDPv6RecvMsgReturn, probes.UDPv6RecvMsgReturnPre470, kv470))
	}

	if (c.CollectUDPv4Conns || c.CollectUDPv6Conns) && (runtimeTracer || coreTracer || kv >= kv470) {
		if err := enableAdvancedUDP(enabled); err != nil {
			return nil, err
		}
	}

	if c.EnableCertCollection {
		// SSL uprobes use a separate UID
		enabled[ssluprobes.IDPairFromFuncName(probes.RawTracepointSchedProcessExit)] = struct{}{}
	}

	return enabled, nil
}

func protocolClassificationTailCalls(cfg *config.Config) []manager.TailCallRoute {
	tcs := []manager.TailCallRoute{
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationTLSClient,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierTLSClientSocketFilter,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationTLSServer,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierTLSServerSocketFilter,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationQueues,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierQueuesSocketFilter,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationDBs,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierDBsSocketFilter,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationGRPC,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierGRPCSocketFilter,
				UID:          probeUID,
			},
		},
	}
	if cfg.CustomBatchingEnabled {
		tcs = append(tcs, manager.TailCallRoute{
			ProgArrayName: probes.TCPCloseProgsMap,
			Key:           0,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.TCPCloseFlushReturn,
				UID:          probeUID,
			},
		})
	}
	return tcs
}

func enableAdvancedUDP(enabled map[manager.ProbeIdentificationPair]struct{}) error {
	missing, err := ebpf.VerifyKernelFuncs("skb_consume_udp", "__skb_free_datagram_locked", "skb_free_datagram_locked")
	if err != nil {
		return fmt.Errorf("error verifying kernel function presence: %s", err)
	}

	if _, miss := missing["skb_consume_udp"]; !miss {
		enableProbe(enabled, probes.SKBConsumeUDP)
	} else if _, miss := missing["__skb_free_datagram_locked"]; !miss {
		enableProbe(enabled, probes.UnderscoredSKBFreeDatagramLocked)
	} else if _, miss := missing["skb_free_datagram_locked"]; !miss {
		enableProbe(enabled, probes.SKBFreeDatagramLocked)
	} else {
		return errors.New("missing desired UDP receive kernel functions")
	}
	return nil
}

func selectVersionBasedProbe(runtimeTracer bool, kv kernel.Version, dfault probes.ProbeFuncName, versioned probes.ProbeFuncName, reqVer kernel.Version) probes.ProbeFuncName {
	if runtimeTracer {
		return dfault
	}
	if kv < reqVer {
		return versioned
	}
	return dfault
}
