// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kprobe

import (
	"fmt"

	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func enableProbe(enabled map[probes.ProbeFuncName]struct{}, name probes.ProbeFuncName) {
	enabled[name] = struct{}{}
}

// enabledProbes returns a map of probes that are enabled per config settings.
// This map does not include the probes used exclusively in the offset guessing process.
func enabledProbes(c *config.Config, runtimeTracer, coreTracer bool) (map[probes.ProbeFuncName]struct{}, error) {
	enabled := make(map[probes.ProbeFuncName]struct{}, 0)

	kv410 := kernel.VersionCode(4, 1, 0)
	kv470 := kernel.VersionCode(4, 7, 0)
	kv5180 := kernel.VersionCode(5, 18, 0)
	kv5190 := kernel.VersionCode(5, 19, 0)
	kv650 := kernel.VersionCode(6, 5, 0)
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, err
	}

	if c.CollectTCPv4Conns || c.CollectTCPv6Conns {
		if ClassificationSupported(c) {
			enableProbe(enabled, probes.ProtocolClassifierEntrySocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierQueuesSocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierDBsSocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierGRPCSocketFilter)
			enableProbe(enabled, probes.NetDevQueue)
			enableProbe(enabled, probes.TCPCloseCleanProtocolsReturn)
		}
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, probes.TCPSendMsg, probes.TCPSendMsgPre410, kv410))
		enableProbe(enabled, probes.TCPSendMsgReturn)
		if kv < kv650 {
			enableProbe(enabled, probes.TCPSendPage)
			enableProbe(enabled, probes.TCPSendPageReturn)
		}
		// 5.19: remove noblock parameter in *_recvmsg https://github.com/torvalds/linux/commit/ec095263a965720e1ca39db1d9c5cd47846c789b
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, selectVersionBasedProbe(runtimeTracer, kv, probes.TCPRecvMsg, probes.TCPRecvMsgPre5190, kv5190), probes.TCPRecvMsgPre410, kv410))
		enableProbe(enabled, probes.TCPRecvMsgReturn)
		enableProbe(enabled, probes.TCPReadSock)
		enableProbe(enabled, probes.TCPReadSockReturn)
		enableProbe(enabled, probes.TCPClose)
		if ((features.HaveMapType(cebpf.RingBuf) == nil) && c.RingbufferEnabled) || runtimeTracer {
			enableProbe(enabled, probes.TCPConnCloseEmitEventRingBuffer)
			enableProbe(enabled, probes.TCPCloseFlushReturn)
		} else {
			enableProbe(enabled, probes.TCPConnCloseEmitEvent)
			enableProbe(enabled, probes.TCPCloseFlushReturnPre580)
		}
		enableProbe(enabled, probes.TCPConnect)
		enableProbe(enabled, probes.TCPFinishConnect)
		enableProbe(enabled, probes.InetCskAcceptReturn)
		enableProbe(enabled, probes.InetCskListenStop)
		// special case for tcp_retransmit_skb probe: on CO-RE,
		// we want to load the version that makes use of
		// the tcp_sock field, which is the same as the
		// runtime compiled implementation
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer || coreTracer, kv, probes.TCPRetransmit, probes.TCPRetransmitPre470, kv470))
		enableProbe(enabled, probes.TCPRetransmitRet)
	}

	if c.CollectUDPv4Conns {
		enableProbe(enabled, probes.UDPDestroySock)
		if ((features.HaveMapType(cebpf.RingBuf) == nil) && c.RingbufferEnabled) || runtimeTracer {
			enableProbe(enabled, probes.UDPDestroySockReturn)
		} else {
			enableProbe(enabled, probes.UDPDestroySockReturnPre580)
		}
		enableProbe(enabled, probes.IPMakeSkb)
		enableProbe(enabled, probes.IPMakeSkbReturn)
		enableProbe(enabled, probes.InetBind)
		enableProbe(enabled, probes.InetBindRet)
		if kv < kv650 {
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
		if ((features.HaveMapType(cebpf.RingBuf) == nil) && c.RingbufferEnabled) || runtimeTracer {
			enableProbe(enabled, probes.UDPv6DestroySockReturn)
		} else {
			enableProbe(enabled, probes.UDPv6DestroySockReturnPre580)
		}
		if kv >= kv5180 || runtimeTracer {
			// prebuilt shouldn't arrive here with 5.18+ and UDPv6 enabled
			if !coreTracer && !runtimeTracer {
				return nil, fmt.Errorf("UDPv6 does not function on prebuilt tracer with kernel versions 5.18+")
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
		if kv < kv650 {
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

	return enabled, nil
}

func enableAdvancedUDP(enabled map[probes.ProbeFuncName]struct{}) error {
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
		return fmt.Errorf("missing desired UDP receive kernel functions")
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
