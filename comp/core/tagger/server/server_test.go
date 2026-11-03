// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type keepAliveTagger struct {
	tagger.Component
	subscription types.Subscription
}

func (t keepAliveTagger) Subscribe(string, *types.Filter) (types.Subscription, error) {
	return t.subscription, nil
}

type keepAliveSubscription struct {
	types.Subscription
	events chan []types.EntityEvent
}

func (s keepAliveSubscription) EventsChan() chan []types.EntityEvent { return s.events }
func (keepAliveSubscription) Unsubscribe()                           {}

type keepAliveStream struct {
	pb.AgentSecure_TaggerStreamEntitiesServer
	ctx     context.Context
	started chan struct{}
	release chan struct{}
}

func (s keepAliveStream) Context() context.Context { return s.ctx }
func (s keepAliveStream) Send(*pb.StreamTagsResponse) error {
	close(s.started)
	<-s.release
	return errors.New("stream closed")
}

func TestKeepAliveErrorAfterHandlerReturns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		events := make(chan []types.EntityEvent)
		stream := keepAliveStream{ctx: t.Context(), started: make(chan struct{}), release: make(chan struct{})}
		server := NewServer(keepAliveTagger{subscription: keepAliveSubscription{events: events}}, telemetrymock.New(t), 1024, 1)
		done := make(chan error, 1)
		go func() { done <- server.TaggerStreamEntities(&pb.StreamTagsRequest{}, stream) }()
		synctest.Wait()
		time.Sleep(streamKeepAliveInterval)
		<-stream.started
		close(events)
		assert.Error(t, <-done)
		close(stream.release)
		// The bubble must finish even though nobody receives the keep-alive error.
		synctest.Wait()
	})
}
