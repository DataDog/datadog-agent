// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remotetaggerimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
)

func TestStart(t *testing.T) {
	grpcServer, authToken, err := grpc.NewMockGrpcSecureServer("5001")
	require.NoError(t, err)
	defer grpcServer.Stop()

	params := tagger.RemoteParams{
		RemoteFilter: types.NewMatchAllFilter(),
		RemoteTarget: func(config.Component) (string, error) { return ":5001", nil },
		RemoteTokenFetcher: func(config.Component) func() (string, error) {
			return func() (string, error) {
				return authToken, nil
			}
		},
	}

	cfg := configmock.New(t)
	log := logmock.New(t)
	telemetry := nooptelemetry.GetCompatComponent()

	remoteTagger, err := NewRemoteTagger(params, cfg, log, telemetry)
	require.NoError(t, err)
	err = remoteTagger.Start(context.TODO())
	require.NoError(t, err)
	remoteTagger.Stop()
}

func TestStartDoNotBlockIfServerIsNotAvailable(t *testing.T) {
	params := tagger.RemoteParams{
		RemoteFilter: types.NewMatchAllFilter(),
		RemoteTarget: func(config.Component) (string, error) { return ":5001", nil },
		RemoteTokenFetcher: func(config.Component) func() (string, error) {
			return func() (string, error) {
				return "something", nil
			}
		},
	}

	cfg := configmock.New(t)
	log := logmock.New(t)
	telemetry := nooptelemetry.GetCompatComponent()

	remoteTagger, err := NewRemoteTagger(params, cfg, log, telemetry)
	require.NoError(t, err)
	err = remoteTagger.Start(context.TODO())
	require.NoError(t, err)
	remoteTagger.Stop()
}
