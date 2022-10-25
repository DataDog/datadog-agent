// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || linux_bpf
// +build windows linux_bpf

package transaction

type HttpTX interface {
	ReqFragment() []byte //
	StatusClass() int
	RequestLatency() float64 //definitely used
	//Method() Method
	StatusCode() uint16   // incomplete_linux
	SetStatusCode(uint16) // incomplete_linux
	StaticTags() uint64
	DynamicTags() []string
	String() string
	Incomplete() bool
	Path(buffer []byte) ([]byte, bool)
	ResponseLastSeen() uint64
	SetResponseLastSeen(ls uint64)
	RequestStarted() uint64
	SetRequestMethod(uint32)
	RequestMethod() uint32 // used in statkeeper
	NewKey(path string, fullPath bool) Key
	NewKeyTuple() KeyTuple
}

// strlen returns the length of a null-terminated string
func strlen(str []byte) int {
	for i := 0; i < len(str); i++ {
		if str[i] == 0 {
			return i
		}
	}
	return len(str)
}
