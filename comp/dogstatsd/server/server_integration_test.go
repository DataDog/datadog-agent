package server

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/pidmapimpl"
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/serverdebugimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStopServer(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fxutil.Test[depsWithoutServer](t, fx.Options(
		core.MockBundle(),
		serverdebugimpl.MockModule(),
		fx.Replace(configComponent.MockParams{
			Overrides: cfg,
		}),
		fx.Supply(Params{Serverless: false}),
		replaymock.MockModule(),
		compressionimpl.MockModule(),
		pidmapimpl.Module(),
		demultiplexerimpl.FakeSamplerMockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	s := newServerCompat(deps.Config, deps.Log, deps.Replay, deps.Debug, false, deps.Demultiplexer, deps.WMeta, deps.PidMap, deps.Telemetry)
	s.start(context.TODO())
	requireStart(t, s)

	s.stop(context.TODO())

	// check that the port can be bound, try for 100 ms
	address, err := net.ResolveUDPAddr("udp", s.UDPLocalAddr())
	require.NoError(t, err, "cannot resolve address")

	for i := 0; i < 10; i++ {
		var conn net.Conn
		conn, err = net.ListenUDP("udp", address)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err, "port is not available, it should be")
}

func TestUDPReceive(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off

	deps := fulfillDepsWithConfigOverride(t, cfg)
	demux := deps.Demultiplexer

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err, "cannot connect to UDP network")
	defer conn.Close()

	testReceive(t, conn, demux)
}

func TestUDPForward(t *testing.T) {
	cfg := make(map[string]interface{})

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	pcHost, pcPort, err := net.SplitHostPort(pc.LocalAddr().String())
	require.NoError(t, err)

	// Setup UDP server to forward to
	cfg["statsd_forward_port"] = pcPort
	cfg["statsd_forward_host"] = pcHost

	// Setup dogstatsd server
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)

	defer pc.Close()

	requireStart(t, deps.Server)

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close()

	// Check if message is forwarded
	message := []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

	_, err = conn.Write(message)
	require.NoError(t, err, "cannot write to DSD socket")

	_ = pc.SetReadDeadline(time.Now().Add(4 * time.Second))

	buffer := make([]byte, len(message))
	_, _, err = pc.ReadFrom(buffer)
	require.NoError(t, err)

	assert.Equal(t, message, buffer)
}

func TestUDSReceiver(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "dsd.socket")

	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off
	cfg["dogstatsd_socket"] = socketPath

	deps := fulfillDepsWithConfigOverride(t, cfg)
	demux := deps.Demultiplexer
	require.True(t, deps.Server.UdsListenerRunning())

	conn, err := net.Dial("unixgram", socketPath)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	testReceive(t, conn, demux)

	s := deps.Server.(*server)
	s.Stop()
	_, err = net.Dial("unixgram", socketPath)
	require.Error(t, err, "UDS listener should be closed")
}

func TestUDSReceiverNoDir(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "nonexistent", "dsd.socket") // nonexistent dir, listener should not be set

	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off
	cfg["dogstatsd_socket"] = socketPath

	deps := fulfillDepsWithConfigOverride(t, cfg)
	require.False(t, deps.Server.UdsListenerRunning())

	_, err := net.Dial("unixgram", socketPath)
	require.Error(t, err, "UDS listener should be closed")
}
