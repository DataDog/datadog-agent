// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
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

func (l *noopGRPCListener) writeEvents(procsToDelete, procsToAdd []*ProcessEntity) {}

var _ mockableGrpcListener = (*grpcListener)(nil)

type getCacheCB func() *pbgo.ProcessStreamResponse

type streamChan chan *pbgo.ProcessStreamResponse

type grpcListener struct {
	wg sync.WaitGroup

	streams      sync.Map
	nextWorkerId uint64

	getCache getCacheCB

	server *grpc.Server

	config config.ConfigReader

	// The address of the server set by start(). Primarily used for testing. May be nil if start() has not been called.
	addr net.Addr
}

func newGrpcListener(config config.ConfigReader, getCache getCacheCB) *grpcListener {
	l := &grpcListener{
		config:   config,
		server:   grpc.NewServer(),
		getCache: getCache,
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

	l.streams.Range(func(key, value any) bool {
		stream := value.(streamChan)
		stream <- &pbgo.ProcessStreamResponse{
			SetEvents:   setEvents,
			UnsetEvents: unsetEvents,
		}
		return true
	})
}

func (l *grpcListener) start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%v", getGRPCStreamPort(l.config)))
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
			_ = log.Error(err)
		}
	}()

	return nil
}

func (l *grpcListener) stop() {
	l.server.Stop()
	l.wg.Wait()
}

func (l *grpcListener) StreamEntities(_ *pbgo.ProcessStreamEntitiesRequest, stream pbgo.ProcessEntityStream_StreamEntitiesServer) error {
	syncMessage := l.getCache()
	err := stream.Send(syncMessage)
	if err != nil {
		return err
	}

	evtsChan := make(streamChan, 1)
	currentWorkerId := l.nextWorkerId
	l.streams.Store(currentWorkerId, evtsChan)
	defer func() {
		l.streams.Delete(currentWorkerId)
		close(evtsChan)
	}()
	l.nextWorkerId++

	var currentEventId int32 = 1
	for evt := range evtsChan {
		evt.EventID = currentEventId
		currentEventId++
		err := stream.Send(evt)
		if err != nil {
			return err
		}
	}

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
