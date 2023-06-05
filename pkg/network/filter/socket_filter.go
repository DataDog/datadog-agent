// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package filter

import (
	"encoding/binary"
	"runtime"

	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/native"
)

type headlessSocketFilter struct {
	fd int
}

func (h *headlessSocketFilter) Close() {
	if h.fd == -1 {
		return
	}
	unix.Close(h.fd)
	h.fd = -1
	runtime.SetFinalizer(h, nil)
}

// HeadlessSocketFilter creates a raw socket attached to the given socket filter.
// The underlying raw socket isn't polled and the filter is not meant to accept any packets.
// The purpose is to use this for pure eBPF packet inspection.
// TODO: After the proof-of-concept we might want to replace the SOCKET_FILTER program by a TC classifier
func HeadlessSocketFilter(cfg *config.Config, filter *manager.Probe) (closeFn func(), err error) {
	hsf := &headlessSocketFilter{}
	ns, err := cfg.GetRootNetNs()
	if err != nil {
		return nil, err
	}
	defer ns.Close()

	err = util.WithNS(ns, func() error {
		hsf.fd, err = unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_ALL)))
		if err != nil {
			return err
		}
		filter.SocketFD = hsf.fd
		runtime.SetFinalizer(hsf, (*headlessSocketFilter).Close)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return func() { hsf.Close() }, nil
}

func htons(a uint16) uint16 {
	var arr [2]byte
	native.Endian.PutUint16(arr[:], a)
	return binary.BigEndian.Uint16(arr[:])
}
