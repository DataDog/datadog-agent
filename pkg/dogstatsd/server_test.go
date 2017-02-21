package dogstatsd

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestNewServer(t *testing.T) {
	s := NewServer(nil)
	defer s.Stop()
	assert.NotNil(t, s)
	assert.True(t, s.Started)
}

func TestStopServer(t *testing.T) {
	s := NewServer(nil)
	s.Stop()

	// check that the port can be bind
	address, _ := net.ResolveUDPAddr("udp", "localhost:8126")
	conn, err := net.ListenUDP("udp", address)
	assert.Nil(t, err)
	conn.Close()
}

func TestUPDReceive(t *testing.T) {
	output := make(chan *aggregator.MetricSample)
	s := NewServer(output)
	defer s.Stop()

	url := fmt.Sprintf("localhost:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, _ := net.Dial("udp", url)
	defer conn.Close()
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))

	res := <-output
	assert.NotNil(t, res)
}
