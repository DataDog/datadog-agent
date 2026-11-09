// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	statsdcomp "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNewMetricsClient(t *testing.T) {
	createErr := errors.New("permission denied")
	tests := []struct {
		name           string
		port           int
		bindHost       string
		createErr      error
		wantErr        string
		wantNoOp       bool
		wantHost       string
		wantPort       int
		wantSameClient bool
	}{
		{
			name:           "uses configured bind host and dogstatsd port",
			port:           8126,
			bindHost:       "127.0.0.1",
			wantHost:       "127.0.0.1",
			wantPort:       8126,
			wantSameClient: true,
		},
		{
			name:      "returns no-op and error when DogStatsD client creation fails",
			port:      8126,
			bindHost:  "127.0.0.1",
			createErr: createErr,
			wantErr:   "failed to create DogStatsD client",
			wantNoOp:  true,
			wantHost:  "127.0.0.1",
			wantPort:  8126,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STATSD_URL", "")
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("dogstatsd_port", tt.port)
			if tt.bindHost != "" {
				mockConfig.SetInTest("bind_host", tt.bindHost)
			}

			wantClient := &statsd.NoOpClient{}
			statsdComp := &recordingStatsdComponent{client: wantClient, err: tt.createErr}
			got, err := NewMetricsClient(New(mockConfig), statsdComp)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.ErrorIs(t, err, tt.createErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantSameClient {
				assert.Same(t, wantClient, got)
			}
			if tt.wantNoOp {
				assert.IsType(t, &statsd.NoOpClient{}, got)
			}
			assert.Equal(t, "host_port", statsdComp.call)
			assert.Equal(t, tt.wantHost, statsdComp.host)
			assert.Equal(t, tt.wantPort, statsdComp.port)
			assert.Empty(t, statsdComp.addr)
		})
	}
}

type recordingStatsdComponent struct {
	client statsd.ClientInterface
	err    error
	call   string
	addr   string
	host   string
	port   int
}

var _ statsdcomp.Component = (*recordingStatsdComponent)(nil)

func (r *recordingStatsdComponent) Get() (statsd.ClientInterface, error) {
	r.call = "get"
	return r.client, r.err
}

func (r *recordingStatsdComponent) Create(_ ...statsd.Option) (statsd.ClientInterface, error) {
	r.call = "create"
	return r.client, r.err
}

func (r *recordingStatsdComponent) CreateForAddr(addr string, _ ...statsd.Option) (statsd.ClientInterface, error) {
	r.call = "addr"
	r.addr = addr
	return r.client, r.err
}

func (r *recordingStatsdComponent) CreateForHostPort(host string, port int, _ ...statsd.Option) (statsd.ClientInterface, error) {
	r.call = "host_port"
	r.host = host
	r.port = port
	return r.client, r.err
}
