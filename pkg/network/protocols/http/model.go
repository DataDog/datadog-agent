// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf
// +build windows,npm linux_bpf

package http

type httpTX interface {
	ReqFragment() []byte
	StatusClass() int
	RequestLatency() float64
	isIPV4() bool
	SrcIPLow() uint64
	SrcIPHigh() uint64
	SrcPort() uint16
	DstIPLow() uint64
	DstIPHigh() uint64
	DstPort() uint16
	Method() Method
	StatusCode() uint16
	SetStatusCode(uint16)
	StaticTags() uint64
	DynamicTags() []string
	String() string
	Incomplete() bool
	Path(buffer []byte) ([]byte, bool)
	ResponseLastSeen() uint64
	SetResponseLastSeen(ls uint64)
	RequestStarted() uint64
	SetRequestMethod(uint32)
	RequestMethod() uint32
}
