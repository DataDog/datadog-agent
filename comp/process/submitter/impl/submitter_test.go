// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package submitterimpl

import (
	"net/http"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	connectionsforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/mock"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	forwarders "github.com/DataDog/datadog-agent/comp/process/forwarders/def"
	forwardersmock "github.com/DataDog/datadog-agent/comp/process/forwarders/mock"
	hostinfomock "github.com/DataDog/datadog-agent/comp/process/hostinfo/mock"
	submitter "github.com/DataDog/datadog-agent/comp/process/submitter/def"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestSubmitterLifecycle(t *testing.T) {
	_ = fxutil.Test[submitter.Component](t, fx.Options(
		hostinfomock.MockModule(),
		core.MockBundle(),
		fx.Provide(func() connectionsforwarder.Component { return connectionsforwardermock.Mock(t) }),
		fx.Provide(func() forwarders.Component { return forwardersmock.New(t) }),
		fx.Provide(func() statsd.ClientInterface {
			return &statsd.NoOpClient{}
		}),
		fxutil.ProvideComponentConstructor(NewComponent),
	))
}

type observedForwarders struct {
	forwarders.Component
	processForwarder defaultforwarder.Component
}

func (f observedForwarders) GetProcessForwarder() defaultforwarder.Component {
	return f.processForwarder
}

type observedProcessForwarder struct {
	defaultforwarder.Component
	submitted chan struct{}
}

func (f observedProcessForwarder) SubmitProcessChecks(payload transaction.BytesPayloads, headers http.Header) (chan defaultforwarder.Response, error) {
	responses, err := f.Component.SubmitProcessChecks(payload, headers)
	close(f.submitted)
	return responses, err
}

func TestSubmitterStopsAfterSubmittingWithMockForwarders(t *testing.T) {
	mockForwarders := forwardersmock.New(t)
	submitted := make(chan struct{})
	f := observedForwarders{
		Component: mockForwarders,
		processForwarder: observedProcessForwarder{
			Component: mockForwarders.GetProcessForwarder(),
			submitted: submitted,
		},
	}
	runner, err := processRunner.NewSubmitter(config.NewMock(t), logmock.New(t), f, &statsd.NoOpClient{}, "test-host", sysprobeconfigmock.NewMock(t))
	require.NoError(t, err)
	s := &submitterImpl{s: runner}
	require.NoError(t, s.Start())
	t.Cleanup(func() {
		stopped := make(chan struct{})
		go func() {
			s.Stop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("submitter Stop blocked after submitting a payload")
		}
	})
	s.Submit(time.Now(), checks.ProcessCheckName, &types.Payload{
		Message: []model.MessageBody{&model.CollectorProc{HostName: "test-host"}},
	})
	// Wait for the forwarder call so shutdown cannot discard the queued payload.
	select {
	case <-submitted:
	case <-time.After(5 * time.Second):
		t.Fatal("payload did not reach the mock forwarder")
	}
}
