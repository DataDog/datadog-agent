// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

func getBufferSize() int {
	return BufferSize
}
func makeTxnFromRequestString(s string) EbpfEvent {
	return EbpfEvent{
		Http: EbpfTx{
			Request_fragment: requestFragment([]byte(s)),
		},
	}
}

func makeTxnFromLatency(lastSeen, started uint64) EbpfEvent {
	return EbpfEvent{
		Http: EbpfTx{
			Response_last_seen: lastSeen,
			Request_started:    started,
		},
	}
}
