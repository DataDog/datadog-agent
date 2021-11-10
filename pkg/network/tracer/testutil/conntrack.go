// +build linux

package testutil

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
)

type delayedConntracker struct {
	netlink.Conntracker

	mux          sync.Mutex
	numDelays    int
	delayPerConn map[string]int
}

// NewDelayedConntracker returns a netlink.Conntracker that returns `nil` for `numDelays`
// consecutive times. After that lookups are routed to the actual Conntracker implementation.
func NewDelayedConntracker(ctr netlink.Conntracker, numDelays int) netlink.Conntracker {
	return &delayedConntracker{
		Conntracker:  ctr,
		numDelays:    numDelays,
		delayPerConn: make(map[string]int),
	}
}

func (ctr *delayedConntracker) GetTranslationForConn(c network.ConnectionStats) *network.IPTranslation {
	ctr.mux.Lock()
	defer ctr.mux.Unlock()

	key, _ := c.ByteKey(make([]byte, 64))
	delays := ctr.delayPerConn[string(key)]
	if delays < ctr.numDelays {
		ctr.delayPerConn[string(key)]++
		return nil
	}

	return ctr.Conntracker.GetTranslationForConn(c)
}
