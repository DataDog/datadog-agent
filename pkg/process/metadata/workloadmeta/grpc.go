// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

type mockableGrpcListener interface {
	writeEvents(procsToDelete, procsToAdd []*ProcessEntity)
}

var _ mockableGrpcListener = (*noopGRPCListener)(nil)

type noopGRPCListener struct{}

func (l *noopGRPCListener) writeEvents(procsToDelete, procsToAdd []*ProcessEntity) {}

var _ mockableGrpcListener = (*grpcListener)(nil)

type grpcListener struct {
	wg sync.WaitGroup

	evts chan *pbgo.ProcessStreamResponse

	streamsLock sync.RWMutex
	streams     []pbgo.ProcessEntityStream_StreamEntitiesServer
}

func newGrpcListener() *grpcListener {
	return &grpcListener{
		evts: make(chan *pbgo.ProcessStreamResponse, 1),
	}
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

func (l *grpcListener) start() {
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
				// Close the streams that have an error when sending
				for _, errIdx := range errIndices {
					l.streams = append(l.streams[:errIdx], l.streams[errIdx+1:]...)
				}
				l.streamsLock.Unlock()
			}
		}
	}()
}

func (l *grpcListener) stop() {
	close(l.evts)
	l.wg.Wait()
}

func (l *grpcListener) StreamEntities(req *pbgo.ProcessStreamResponse, stream pbgo.ProcessEntityStream_StreamEntitiesServer) error {
	l.streamsLock.Lock()
	defer l.streamsLock.Unlock()

	l.streams = append(l.streams, stream)
	return nil
}
