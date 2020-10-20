package network

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
		packetSrc *packetSource
		srcErr    error
	)

	nsErr := util.WithRootNS(rootPath, func() {
		packetSrc, srcErr = newPacketSource(filter)
	})
	if nsErr != nil {
		return nil, nsErr
	}
	if srcErr != nil {
		return nil, srcErr
	}

	return func() { packetSrc.Close() }, nil
}
