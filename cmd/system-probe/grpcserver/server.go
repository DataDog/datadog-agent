package grpcserver

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding"
	"github.com/DataDog/datadog-agent/pkg/proto/test2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
	"time"
)

const inactivityRestartDuration = 20 * time.Minute

type SystemProbeServer struct{}

func writeConnectionsTogRPC(marshaler encoding.Marshaler, cs *network.Connections) ([]byte, error) {
	defer network.Reclaim(cs)

	buf, err := marshaler.Marshal(cs)
	if err != nil {
		log.Errorf("unable to marshall connections with type %s: %s", marshaler.ContentType(), err)
		return nil, err
	}

	log.Tracef("/connections: %d connections, %d bytes", len(cs.Conns), len(buf))
	return buf, nil
}

func logRequests(client string, count uint64, connectionsCount int, start time.Time) {
	args := []interface{}{client, count, connectionsCount, time.Now().Sub(start)}
	msg := "Got request on /connections?client_id=%s (count: %d): retrieved %d connections in %s"
	switch {
	case count <= 5, count%20 == 0:
		log.Infof(msg, args...)
	default:
		log.Debugf(msg, args...)
	}
}

func (s *SystemProbeServer) GetConnections(req *test2.GetConnectionsRequest, s2 test2.SystemProbe_GetConnectionsServer) error {
	start := time.Now()
	var runCounter = atomic.NewUint64(0)
	id := req.GetClientID()
	tracer, timer := modules.GetNetworkTracerTracerAndRestartTimer()
	cs, err := tracer.GetActiveConnections(id)
	if err != nil {
		log.Errorf("unable to retrieve connections: %s", err)
		return err
	}

	marshaler := encoding.GetMarshaler(encoding.ContentTypeProtobuf)
	cs2, err := writeConnectionsTogRPC(marshaler, cs)
	if err != nil {
		log.Errorf("unable to writeConnectionsTogRPC: %s", err)
		return err
	}

	if timer != nil {
		timer.Reset(inactivityRestartDuration)
	}
	count := runCounter.Inc()
	logRequests(id, count, len(cs.Conns), start)

	//	iterate over all the connections
	s2.Send(&test2.Connection{Data: cs2})
	return nil
}
