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
	s, err := NewServer(nil, nil, nil)
	assert.Nil(t, err)
	defer s.Stop()
	assert.NotNil(t, s)
	assert.True(t, s.Started)
}

func TestStopServer(t *testing.T) {
	s, err := NewServer(nil, nil, nil)
	assert.Nil(t, err)
	s.Stop()

	// check that the port can be bind
	address, _ := net.ResolveUDPAddr("udp", "localhost:8126")
	conn, err := net.ListenUDP("udp", address)
	assert.Nil(t, err)
	conn.Close()
}

func TestUPDReceive(t *testing.T) {
	metricOut := make(chan *aggregator.MetricSample)
	eventOut := make(chan aggregator.Event)
	serviceOut := make(chan aggregator.ServiceCheck)
	s, err := NewServer(metricOut, eventOut, serviceOut)
	assert.Nil(t, err)
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	assert.Nil(t, err)
	defer conn.Close()

	// Test metric
	conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.NotNil(t, res)
		assert.Equal(t, res.Name, "daemon")
		assert.EqualValues(t, res.Value, 666.0)
		assert.Equal(t, res.Mtype, aggregator.GaugeType)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	conn.Write([]byte("daemon:666|c|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.NotNil(t, res)
		assert.Equal(t, res.Name, "daemon")
		assert.EqualValues(t, res.Value, 666.0)
		assert.Equal(t, aggregator.CounterType, res.Mtype)
		assert.Equal(t, 0.5, res.SampleRate)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	conn.Write([]byte("daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.NotNil(t, res)
		assert.Equal(t, res.Name, "daemon")
		assert.EqualValues(t, res.Value, 666.0)
		assert.Equal(t, aggregator.HistogramType, res.Mtype)
		assert.Equal(t, 0.5, res.SampleRate)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	conn.Write([]byte("daemon:666|ms|@0.5|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.NotNil(t, res)
		assert.Equal(t, res.Name, "daemon")
		assert.EqualValues(t, res.Value, 666.0)
		assert.Equal(t, aggregator.HistogramType, res.Mtype)
		assert.Equal(t, 0.5, res.SampleRate)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	conn.Write([]byte("daemon_set:abc|s|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.NotNil(t, res)
		assert.Equal(t, res.Name, "daemon_set")
		assert.Equal(t, res.RawValue, "abc")
		assert.Equal(t, res.Mtype, aggregator.SetType)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous metric
	conn.Write([]byte("daemon1:666:777|g\ndaemon2:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	select {
	case res := <-metricOut:
		assert.NotNil(t, res)
		assert.Equal(t, res.Name, "daemon2")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test Service Check
	conn.Write([]byte("_sc|agent.up|0|d:12345|h:localhost|m:this is fine|#sometag1:somevalyyue1,sometag2:somevalue2"))
	select {
	case res := <-serviceOut:
		assert.NotNil(t, res)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous Service Check
	conn.Write([]byte("_sc|agen.down\n_sc|agent.up|0|d:12345|h:localhost|m:this is fine|#sometag1:somevalyyue1,sometag2:somevalue2"))
	select {
	case res := <-serviceOut:
		assert.NotNil(t, res)
		assert.Equal(t, res.CheckName, "agent.up")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test Event
	conn.Write([]byte("_e{10,10}:test title|test\\ntext|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))
	select {
	case res := <-eventOut:
		assert.NotNil(t, res)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

	// Test erroneous Event
	conn.Write([]byte("_e{10,0}:test title|\n_e{11,10}:test title2|test\\ntext|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))
	select {
	case res := <-eventOut:
		assert.NotNil(t, res)
		assert.Equal(t, res.Title, "test title2")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}
