// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"net"
	"strconv"
	"sync"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GRPCServer implements a gRPC server to expose Process Entities collected with a WorkloadMetaExtractor
type GRPCServer struct {
	config    config.ConfigReader
	extractor *WorkloadMetaExtractor
	server    *grpc.Server
	// The address of the server set by start(). Primarily used for testing. May be nil if start() has not been called.
	addr net.Addr

	wg sync.WaitGroup
}

// NewGRPCServer creates a new instance of a GRPCServer
func NewGRPCServer(config config.ConfigReader, extractor *WorkloadMetaExtractor) *GRPCServer {
	l := &GRPCServer{
		config:    config,
		extractor: extractor,
		server:    grpc.NewServer(),
	}

	pbgo.RegisterProcessEntityStreamServer(l.server, l)
	return l
}

func (l *GRPCServer) consumeProcessDiff(diff *ProcessCacheDiff) ([]*pbgo.ProcessEventSet, []*pbgo.ProcessEventUnset) {
	setEvents := make([]*pbgo.ProcessEventSet, len(diff.creation))
	for i, proc := range diff.creation {
		setEvents[i] = &pbgo.ProcessEventSet{
			Pid:      proc.pid,
			Language: &pbgo.Language{Name: string(proc.language.Name)},
		}
	}

	unsetEvents := make([]*pbgo.ProcessEventUnset, len(diff.deletion))
	for i, proc := range diff.deletion {
		unsetEvents[i] = &pbgo.ProcessEventUnset{Pid: proc.pid}
	}

	return setEvents, unsetEvents
}

// Start starts the GRPCServer to listen for new connections
func (l *GRPCServer) Start() error {
	log.Info("Starting Process Entity WorkloadMeta gRPC server")
	listener, err := getListener(l.config)
	if err != nil {
		return err
	}
	l.addr = listener.Addr()
	log.Info("Process Entity WorkloadMeta gRPC server has started listening on", listener.Addr().String())

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()

		err = l.server.Serve(listener)
		if err != nil {
			log.Error(err)
		}
	}()

	return nil
}

// Stop stops and cleans up resources allocated by the GRPCServer
func (l *GRPCServer) Stop() {
	log.Info("Stopping Process Entity WorkloadMeta gRPC server")
	l.server.Stop()
	l.wg.Wait()
	log.Info("Process Entity WorkloadMeta gRPC server stopped")
}

// StreamEntities streams Process Entities collected through the WorkloadMetaExtractor
func (l *GRPCServer) StreamEntities(_ *pbgo.ProcessStreamEntitiesRequest, out pbgo.ProcessEntityStream_StreamEntitiesServer) error {
	// When connection is created, send a snapshot of all processes detected on the host so far as "SET" events
	procs, snapshotVersion := l.extractor.GetAllProcessEntities()
	setEvents := make([]*pbgo.ProcessEventSet, 0, len(procs))
	for _, proc := range procs {
		setEvents = append(setEvents, &pbgo.ProcessEventSet{
			Pid:      proc.pid,
			Language: &pbgo.Language{Name: string(proc.language.Name)},
		})
	}

	syncMessage := &pbgo.ProcessStreamResponse{
		EventID:   snapshotVersion,
		SetEvents: setEvents,
	}
	err := out.Send(syncMessage)
	if err != nil {
		log.Warnf("error sending process entity event: %s", err)
		return err
	}

	// Once connection is established, only diffs (process creations/deletions) are sent to the client
	for {
		select {
		case diff := <-l.extractor.ProcessCacheDiff():
			// Do not send diff if it has the same or older version of the cache snapshot sent on the connection creation
			if diff.cacheVersion <= snapshotVersion {
				continue
			}

			sets, unsets := l.consumeProcessDiff(diff)
			msg := &pbgo.ProcessStreamResponse{
				EventID:     diff.cacheVersion,
				SetEvents:   sets,
				UnsetEvents: unsets,
			}
			err := out.Send(msg)
			if err != nil {
				log.Warnf("error sending process entity event: %s", err)
				return err
			}

		case <-out.Context().Done():
			return nil
		}
	}
}

// getListener returns a listening connection
func getListener(cfg config.ConfigReader) (net.Listener, error) {
	host, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	address := net.JoinHostPort(host, strconv.Itoa(getGRPCStreamPort(cfg)))
	return net.Listen("tcp", address)
}

func getGRPCStreamPort(cfg config.ConfigReader) int {
	grpcPort := cfg.GetInt("process_config.language_detection.grpc_port")
	if grpcPort <= 0 {
		log.Warnf("Invalid process_config.language_detection.grpc_port -- %d, using default port %d", grpcPort, config.DefaultProcessEntityStreamPort)
		grpcPort = config.DefaultProcessEntityStreamPort
	}
	return grpcPort
}
