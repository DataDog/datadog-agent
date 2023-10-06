package listeners

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

func udsDatagramListenerFactory(packetOut chan packets.Packets, manager *packets.PoolManager, cfg config.Component) (StatsdListener, error) {
	return NewUDSDatagramListener(packetOut, manager, nil, cfg, nil)
}

func TestNewUDSDatagramListener(t *testing.T) {
	testNewUDSListener(t, udsDatagramListenerFactory, "unixgram")
}

func TestStartStopUDSDatagramListener(t *testing.T) {
	testStartStopUDSListener(t, udsDatagramListenerFactory, "unixgram")
}

func TestUDSDatagramReceive(t *testing.T) {
	testUDSReceive(t, udsDatagramListenerFactory, "unixgram")
}
