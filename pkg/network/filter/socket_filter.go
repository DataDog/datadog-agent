// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package filter

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/ebpf/manager"
)

// HeadlessSocketFilter creates a raw socket attached to the given socket filter.
// The underlying raw socket isn't polled and the filter is not meant to accept any packets.
// The purpose is to use this for pure eBPF packet inspection.
// TODO: After the proof-of-concept we might want to replace the SOCKET_FILTER program by a TC classifier
func HeadlessSocketFilter(rootPath string, filter *manager.Probe) (closeFn func(), err error) {
	var (
		packetSrc *AFPacketSource
		srcErr    error
	)

	err = util.WithRootNS(rootPath, func() error {
		packetSrc, srcErr = NewPacketSource(filter)
		return srcErr
	})
	if err != nil {
		return nil, err
	}

	return func() { packetSrc.Close() }, nil
}
