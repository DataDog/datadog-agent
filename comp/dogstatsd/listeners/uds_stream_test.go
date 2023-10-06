package listeners

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

func udsStreamListenerFactory(packetOut chan packets.Packets, manager *packets.PoolManager, cfg config.Component) (StatsdListener, error) {
	return NewUDSStreamListener(packetOut, manager, nil, cfg, nil)
}

func TestNewUDSStreamListener(t *testing.T) {
	testNewUDSListener(t, udsStreamListenerFactory, "unix")
}

func TestStartStopUDSStreamListener(t *testing.T) {
	testStartStopUDSListener(t, udsStreamListenerFactory, "unix")
}

func TestUDSStreamReceive(t *testing.T) {
	testUDSReceive(t, udsStreamListenerFactory, "unix")
}
