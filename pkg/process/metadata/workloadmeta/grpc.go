// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type mockableGrpcListener interface {
	writeEvents(procsToDelete, procsToAdd []*ProcessEntity)
}

var _ mockableGrpcListener = (*noopGRPCListener)(nil)

type noopGRPCListener struct{}

func (l *noopGRPCListener) writeEvents(_, _ []*ProcessEntity) {}

var _ mockableGrpcListener = (*grpcListener)(nil)

type getCacheCB func() *pbgo.ProcessStreamResponse

type grpcListener struct {
	wg sync.WaitGroup

	evts chan *pbgo.ProcessStreamResponse

	streamsLock sync.RWMutex
	streams     []pbgo.ProcessEntityStream_StreamEntitiesServer

	getCache getCacheCB

	server *grpc.Server

	config config.ConfigReader
}

func newGrpcListener(config config.ConfigReader, getCache getCacheCB) *grpcListener {
	l := &grpcListener{
		config:   config,
		server:   grpc.NewServer(),
		getCache: getCache,
		evts:     make(chan *pbgo.ProcessStreamResponse, 1),
	}

	pbgo.RegisterProcessEntityStreamServer(l.server, l)
	return l
}

func (l *grpcListener) writeEvents(procsToDelete, procsToAdd []*ProcessEntity) {
	setEvents := make([]*pbgo.ProcessEventSet, len(procsToAdd))
	for i, proc := range procsToAdd {
		setEvents[i] = &pbgo.ProcessEventSet{
			Pid:      proc.pid,
			Language: &pbgo.Language{Name: string(proc.language.Name)},
		}
	}

	unsetEvents := make([]*pbgo.ProcessEventUnset, len(procsToDelete))
	for i, proc := range procsToDelete {
		unsetEvents[i] = &pbgo.ProcessEventUnset{Pid: proc.pid}
	}

	l.evts <- &pbgo.ProcessStreamResponse{
		SetEvents:   setEvents,
		UnsetEvents: unsetEvents,
	}
}

func (l *grpcListener) start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%v", getGRPCStreamPort(l.config)))
	if err != nil {
		return err
	}

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()

		err = l.server.Serve(listener)
		if errors.Is(err, grpc.ErrServerStopped) {
			log.Info("The WorkloadMeta gRPC server has stopped")
		} else if err != nil {
			_ = log.Error(err)
		}

		err = listener.Close()
		if err != nil {
			_ = log.Error("Failed to close listener", listener)
		}
	}()

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for evt := range l.evts {
			var errIndices []int

			l.streamsLock.RLock()
			for i, stream := range l.streams {
				err := stream.Send(evt)
				if err != nil {
					errIndices = append(errIndices, i)
				}
			}
			l.streamsLock.RUnlock()

			// Most of the time we should have no errors
			if len(errIndices) > 0 {
				l.streamsLock.Lock()
				// Clean up the streams that have an error when sending
				for _, errIdx := range errIndices {
					l.streams = append(l.streams[:errIdx], l.streams[errIdx+1:]...)
				}
				l.streamsLock.Unlock()
			}
		}
	}()

	return nil
}

func (l *grpcListener) stop() {
	close(l.evts)
	l.wg.Wait()
}

func (l *grpcListener) StreamEntities(_ *pbgo.ProcessStreamEntitiesRequest, stream pbgo.ProcessEntityStream_StreamEntitiesServer) error {
	l.streamsLock.Lock()
	defer l.streamsLock.Unlock()

	syncMessage := l.getCache()
	err := stream.Send(syncMessage)
	if err != nil {
		return err
	}

	l.streams = append(l.streams, stream)

	return nil
}

func getGRPCStreamPort(cfg config.ConfigReader) int {
	grpcPort := cfg.GetInt("process_config.language_detection.grpc_port")
	if grpcPort <= 0 {
		_ = log.Warnf("Invalid process_config.expvar_port -- %d, using default port %d", grpcPort, config.DefaultProcessEntityStreamPort)
		grpcPort = config.DefaultProcessEntityStreamPort
	}
	return grpcPort
}
