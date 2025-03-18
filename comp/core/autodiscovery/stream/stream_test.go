// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stream

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/metadata"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	noopautoconfig "github.com/DataDog/datadog-agent/comp/core/autodiscovery/noopimpl"
	autodiscoveryscheduler "github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func getAutodiscoveryNoop(t *testing.T) autodiscovery.Component {
	return fxutil.Test[autodiscovery.Component](
		t,
		noopautoconfig.Module(),
	)
}

type acMock struct {
	autodiscovery.Component

	addScheduler func(string, autodiscoveryscheduler.Scheduler, bool)
}

func (ac *acMock) AddScheduler(name string, scheduler autodiscoveryscheduler.Scheduler, replay bool) {
	ac.addScheduler(name, scheduler, replay)
}

type outMock struct {
	send func(*pb.AutodiscoveryStreamResponse) error
	ctx  context.Context
}

func (out *outMock) Send(resp *pb.AutodiscoveryStreamResponse) error {
	return out.send(resp)
}

func (out *outMock) SetHeader(metadata.MD) error {
	panic("not implemented")
}

func (out *outMock) SendHeader(metadata.MD) error {
	panic("not implemented")
}

func (out *outMock) SetTrailer(metadata.MD) {
	panic("not implemented")
}

func (out *outMock) Context() context.Context {
	return out.ctx
}

func (out *outMock) SendMsg(any) error {
	panic("not implemented")
}

func (out *outMock) RecvMsg(any) error {
	panic("not implemented")
}

func setupTestConfig(t *testing.T, sendErr error) (chan error, chan autodiscoveryscheduler.Scheduler, chan *pb.AutodiscoveryStreamResponse, context.CancelFunc) {
	schedulerChan := make(chan autodiscoveryscheduler.Scheduler, 1)
	sendChan := make(chan *pb.AutodiscoveryStreamResponse, 1)

	acNoop := getAutodiscoveryNoop(t)
	ac := &acMock{
		acNoop,
		func(_ string, scheduler autodiscoveryscheduler.Scheduler, _ bool) {
			schedulerChan <- scheduler
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	out := &outMock{
		send: func(resp *pb.AutodiscoveryStreamResponse) error {
			sendChan <- resp
			return sendErr
		},
		ctx: ctx,
	}

	configErrChan := make(chan error)
	go func() {
		configErrChan <- Config(ac, out)
	}()

	return configErrChan, schedulerChan, sendChan, cancel
}

func TestConfig(t *testing.T) {
	configs := []integration.Config{
		{
			Name: "test",
		},
	}

	t.Run("schedule unschedule", func(t *testing.T) {
		configErrChan, schedulerChan, sendChan, cancel := setupTestConfig(t, nil)

		scheduler := <-schedulerChan
		require.NotNil(t, scheduler)

		scheduler.Schedule(configs)
		sent := <-sendChan
		require.NotNil(t, sent)
		require.Len(t, sent.Configs, 1)
		require.Equal(t, "test", sent.Configs[0].Name)
		require.Equal(t, pb.ConfigEventType_SCHEDULE, sent.Configs[0].EventType)

		scheduler.Unschedule(configs)
		sent = <-sendChan
		require.NotNil(t, sent)
		require.Len(t, sent.Configs, 1)
		require.Equal(t, "test", sent.Configs[0].Name)
		require.Equal(t, pb.ConfigEventType_UNSCHEDULE, sent.Configs[0].EventType)

		cancel()

		require.NoError(t, <-configErrChan)
	})

	t.Run("send error", func(t *testing.T) {
		sendError := errors.New("send error")
		configErrChan, schedulerChan, sendChan, _ := setupTestConfig(t, sendError)

		scheduler := <-schedulerChan
		require.NotNil(t, scheduler)

		scheduler.Schedule(configs)
		sent := <-sendChan
		require.NotNil(t, sent)
		require.Len(t, sent.Configs, 1)
		require.Equal(t, "test", sent.Configs[0].Name)
		require.Equal(t, pb.ConfigEventType_SCHEDULE, sent.Configs[0].EventType)

		require.ErrorIs(t, <-configErrChan, sendError)
	})

	t.Run("multiple errors", func(t *testing.T) {
		sendError := errors.New("send error")
		configErrChan, schedulerChan, sendChan, _ := setupTestConfig(t, sendError)

		scheduler := <-schedulerChan
		require.NotNil(t, scheduler)

		scheduler.Schedule(configs)
		<-sendChan
		scheduler.Unschedule(configs)
		<-sendChan

		require.ErrorIs(t, <-configErrChan, sendError)
	})
}
