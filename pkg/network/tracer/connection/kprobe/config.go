// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"fmt"

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
func enabledProbes(c *config.Config, runtimeTracer bool) (map[probes.ProbeFuncName]struct{}, error) {
	enabled := make(map[probes.ProbeFuncName]struct{}, 0)

	kv410 := kernel.VersionCode(4, 1, 0)
	kv470 := kernel.VersionCode(4, 7, 0)
	kv5190 := kernel.VersionCode(5, 19, 0)
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, err
	}

	if c.CollectTCPv4Conns || c.CollectTCPv6Conns {
		if ClassificationSupported(c) {
			enableProbe(enabled, probes.ProtocolClassifierEntrySocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierQueuesSocketFilter)
			enableProbe(enabled, probes.ProtocolClassifierDBsSocketFilter)
			enableProbe(enabled, probes.NetDevQueue)
		}
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, probes.TCPSendMsg, probes.TCPSendMsgPre410, kv410))
		enableProbe(enabled, probes.TCPSendMsgReturn)
		enableProbe(enabled, probes.TCPSendPage)
		enableProbe(enabled, probes.TCPSendPageReturn)
		// 5.19: remove noblock parameter in *_recvmsg https://github.com/torvalds/linux/commit/ec095263a965720e1ca39db1d9c5cd47846c789b
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, selectVersionBasedProbe(runtimeTracer, kv, probes.TCPRecvMsg, probes.TCPRecvMsgPre5190, kv5190), probes.TCPRecvMsgPre410, kv410))
		enableProbe(enabled, probes.TCPRecvMsgReturn)
		enableProbe(enabled, probes.TCPReadSock)
		enableProbe(enabled, probes.TCPReadSockReturn)
		enableProbe(enabled, probes.TCPClose)
		enableProbe(enabled, probes.TCPCloseReturn)
		enableProbe(enabled, probes.TCPConnect)
		enableProbe(enabled, probes.TCPFinishConnect)
		enableProbe(enabled, probes.InetCskAcceptReturn)
		enableProbe(enabled, probes.InetCskListenStop)
		enableProbe(enabled, probes.TCPSetState)
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, probes.TCPRetransmit, probes.TCPRetransmitPre470, kv470))
		enableProbe(enabled, probes.TCPRetransmitRet)

		missing, err := ebpf.VerifyKernelFuncs("sockfd_lookup_light")
		if err == nil && len(missing) == 0 {
			enableProbe(enabled, probes.SockFDLookup)
			enableProbe(enabled, probes.SockFDLookupRet)
		}
	}

	if c.CollectUDPv4Conns {
		enableProbe(enabled, probes.UDPDestroySock)
		enableProbe(enabled, probes.UDPDestroySockReturn)
		enableProbe(enabled, probes.IPMakeSkb)
		enableProbe(enabled, probes.IPMakeSkbReturn)
		enableProbe(enabled, probes.InetBind)
		enableProbe(enabled, probes.InetBindRet)
		enableProbe(enabled, probes.UDPSendPage)
		enableProbe(enabled, probes.UDPSendPageReturn)
		if kv >= kv5190 || runtimeTracer {
			enableProbe(enabled, probes.UDPRecvMsg)
		} else if kv >= kv470 {
			enableProbe(enabled, probes.UDPRecvMsgPre5190)
		} else if kv >= kv410 {
			enableProbe(enabled, probes.UDPRecvMsgPre470)
		} else {
			enableProbe(enabled, probes.UDPRecvMsgPre410)
		}
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, probes.UDPRecvMsgReturn, probes.UDPRecvMsgReturnPre470, kv470))
	}

	if c.CollectUDPv6Conns {
		enableProbe(enabled, probes.UDPv6DestroySock)
		enableProbe(enabled, probes.UDPv6DestroySockReturn)
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, probes.IP6MakeSkb, probes.IP6MakeSkbPre470, kv470))
		enableProbe(enabled, probes.IP6MakeSkbReturn)
		enableProbe(enabled, probes.Inet6Bind)
		enableProbe(enabled, probes.Inet6BindRet)
		enableProbe(enabled, probes.UDPSendPage)
		enableProbe(enabled, probes.UDPSendPageReturn)
		if kv >= kv5190 || runtimeTracer {
			enableProbe(enabled, probes.UDPv6RecvMsg)
		} else if kv >= kv470 {
			enableProbe(enabled, probes.UDPv6RecvMsgPre5190)
		} else if kv >= kv410 {
			enableProbe(enabled, probes.UDPv6RecvMsgPre470)
		} else {
			enableProbe(enabled, probes.UDPv6RecvMsgPre410)
		}
		enableProbe(enabled, selectVersionBasedProbe(runtimeTracer, kv, probes.UDPv6RecvMsgReturn, probes.UDPv6RecvMsgReturnPre470, kv470))
	}

	if (c.CollectUDPv4Conns || c.CollectUDPv6Conns) && (runtimeTracer || kv >= kv470) {
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
