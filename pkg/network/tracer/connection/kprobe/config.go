// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func enableProbe(enabled map[probes.ProbeName]string, name probes.ProbeName) {
	if fn, ok := mainProbes[name]; ok {
		enabled[name] = fn
		return
	}
	if fn, ok := altProbes[name]; ok {
		enabled[name] = fn
	}
}

// enabledProbes returns a map of probes that are enabled per config settings.
// This map does not include the probes used exclusively in the offset guessing process.
func enabledProbes(c *config.Config, runtimeTracer bool) (map[probes.ProbeName]string, error) {
	enabled := make(map[probes.ProbeName]string, 0)

	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, err
	}
	pre410Kernel := kv < kernel.VersionCode(4, 1, 0)

	if c.CollectTCPConns {
		if !runtimeTracer && pre410Kernel {
			enableProbe(enabled, probes.TCPSendMsgPre410)
		} else {
			enableProbe(enabled, probes.TCPSendMsg)
		}
		enableProbe(enabled, probes.TCPCleanupRBuf)
		enableProbe(enabled, probes.TCPClose)
		enableProbe(enabled, probes.TCPCloseReturn)
		enableProbe(enabled, probes.InetCskAcceptReturn)
		enableProbe(enabled, probes.InetCskListenStop)
		enableProbe(enabled, probes.TCPSetState)

		if !runtimeTracer && kv < kernel.VersionCode(4, 7, 0) {
			enableProbe(enabled, probes.TCPRetransmitPre470)
		} else {
			enableProbe(enabled, probes.TCPRetransmit)
		}

		missing, err := ebpf.VerifyKernelFuncs(filepath.Join(c.ProcRoot, "kallsyms"), []string{"sockfd_lookup_light"})
		if err == nil && len(missing) == 0 {
			enableProbe(enabled, probes.SockFDLookup)
			enableProbe(enabled, probes.SockFDLookupRet)
			enableProbe(enabled, probes.DoSendfile)
			enableProbe(enabled, probes.DoSendfileRet)
		}
	}

	if c.CollectUDPConns {
		enableProbe(enabled, probes.UDPRecvMsgReturn)
		enableProbe(enabled, probes.UDPDestroySock)
		enableProbe(enabled, probes.UDPDestroySockReturn)
		enableProbe(enabled, probes.IPMakeSkb)
		enableProbe(enabled, probes.InetBind)
		enableProbe(enabled, probes.InetBindRet)

		if c.CollectIPv6Conns {
			if !runtimeTracer && kv < kernel.VersionCode(4, 7, 0) {
				enableProbe(enabled, probes.IP6MakeSkbPre470)
			} else {
				enableProbe(enabled, probes.IP6MakeSkb)
			}

			enableProbe(enabled, probes.Inet6Bind)
			enableProbe(enabled, probes.Inet6BindRet)
		}

		if !runtimeTracer && pre410Kernel {
			enableProbe(enabled, probes.UDPRecvMsgPre410)
		} else {
			enableProbe(enabled, probes.UDPRecvMsg)
		}
	}

	return enabled, nil
}
