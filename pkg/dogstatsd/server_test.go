package dogstatsd

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestNewServer(t *testing.T) {
	s, err := NewServer(nil)
	assert.Nil(t, err)
	defer s.Stop()
	assert.NotNil(t, s)
	assert.True(t, s.Started)
}

func TestStopServer(t *testing.T) {
	s, err := NewServer(nil)
	assert.Nil(t, err)
	s.Stop()

	// check that the port can be bind
	address, _ := net.ResolveUDPAddr("udp", "localhost:8126")
	conn, err := net.ListenUDP("udp", address)
	assert.Nil(t, err)
	conn.Close()
}

func TestUPDReceive(t *testing.T) {
	output := make(chan *aggregator.MetricSample)
	s, err := NewServer(output)
	assert.Nil(t, err)
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	assert.Nil(t, err)
	defer conn.Close()
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))

	select {
	case res := <-output:
		assert.NotNil(t, res)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

}
