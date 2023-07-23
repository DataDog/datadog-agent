// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package utils

import "sync"

type CallbackRecorder struct {
	mux           sync.Mutex
	ReturnError   error
	callsByPathID map[PathIdentifier]int
}

func (r *CallbackRecorder) Fn() func(FilePath) error {
	return func(f FilePath) error {
		r.mux.Lock()
		defer r.mux.Unlock()

		if r.callsByPathID == nil {
			r.callsByPathID = make(map[PathIdentifier]int)
		}

		r.callsByPathID[f.ID]++

		return r.ReturnError
	}
}

func (r *CallbackRecorder) CallsForPathID(pathID PathIdentifier) int {
	r.mux.Lock()
	defer r.mux.Unlock()

	return r.callsByPathID[pathID]
}
