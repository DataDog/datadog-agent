// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp && test

package run

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

var errNoHostname = errors.New("unable to reliably determine the host name")

// fakeHostname is a hostname component returning a fixed result and counting calls.
type fakeHostname struct {
	data  hostnameinterface.Data
	err   error
	calls int
}

func (f *fakeHostname) Get(ctx context.Context) (string, error) {
	data, err := f.GetWithProvider(ctx)
	return data.Hostname, err
}

func (f *fakeHostname) GetSafe(ctx context.Context) string {
	name, err := f.Get(ctx)
	if err != nil {
		return "unknown host"
	}
	return name
}

func (f *fakeHostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	f.calls++
	return f.data, f.err
}

// setKubernetes makes pkgconfigenv.IsKubernetes return the given value.
func setKubernetes(t *testing.T, inKubernetes bool) {
	if inKubernetes {
		t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	} else {
		t.Setenv("KUBERNETES_SERVICE_PORT", "")
	}
	t.Setenv("KUBERNETES", "")
}

func newTestStandaloneHostname(cfg coreconfig.Component, chain *fakeHostname, instanceID string, instanceIDErr error) (*standaloneHostname, *int) {
	ec2Calls := 0
	return &standaloneHostname{
		Component: chain,
		cfg:       cfg,
		getInstanceID: func(context.Context) (string, error) {
			ec2Calls++
			return instanceID, instanceIDErr
		},
	}, &ec2Calls
}

func TestStandaloneHostname(t *testing.T) {
	ctx := context.Background()

	t.Run("provider chain succeeds", func(t *testing.T) {
		setKubernetes(t, true)
		chain := &fakeHostname{data: hostnameinterface.Data{Hostname: "ip-10-0-0-1.ec2.internal", Provider: "container"}}
		h, ec2Calls := newTestStandaloneHostname(coreconfig.NewMock(t), chain, "i-0123456789abcdef0", nil)

		data, err := h.GetWithProvider(ctx)
		require.NoError(t, err)
		assert.Equal(t, hostnameinterface.Data{Hostname: "ip-10-0-0-1.ec2.internal", Provider: "container"}, data)
		assert.Zero(t, *ec2Calls)
	})

	t.Run("falls back to the EC2 instance ID in Kubernetes without kubelet host", func(t *testing.T) {
		setKubernetes(t, true)
		chain := &fakeHostname{err: errNoHostname}
		h, ec2Calls := newTestStandaloneHostname(coreconfig.NewMock(t), chain, "i-0123456789abcdef0", nil)

		data, err := h.GetWithProvider(ctx)
		require.NoError(t, err)
		assert.Equal(t, hostnameinterface.Data{Hostname: "i-0123456789abcdef0", Provider: "aws"}, data)

		// The fallback is kept, without running the provider chain or IMDS again.
		name, err := h.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "i-0123456789abcdef0", name)
		assert.Equal(t, "i-0123456789abcdef0", h.GetSafe(ctx))
		assert.Equal(t, 1, chain.calls)
		assert.Equal(t, 1, *ec2Calls)
	})

	t.Run("no fallback when the kubelet host is configured", func(t *testing.T) {
		setKubernetes(t, true)
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"kubernetes_kubelet_host": "10.0.0.1"})
		chain := &fakeHostname{err: errNoHostname}
		h, ec2Calls := newTestStandaloneHostname(cfg, chain, "i-0123456789abcdef0", nil)

		_, err := h.Get(ctx)
		assert.ErrorIs(t, err, errNoHostname)
		assert.Equal(t, "unknown host", h.GetSafe(ctx))
		assert.Zero(t, *ec2Calls)
	})

	t.Run("no fallback outside Kubernetes", func(t *testing.T) {
		setKubernetes(t, false)
		chain := &fakeHostname{err: errNoHostname}
		h, ec2Calls := newTestStandaloneHostname(coreconfig.NewMock(t), chain, "i-0123456789abcdef0", nil)

		_, err := h.Get(ctx)
		assert.ErrorIs(t, err, errNoHostname)
		assert.Zero(t, *ec2Calls)
	})

	t.Run("original error when IMDS is not reachable", func(t *testing.T) {
		setKubernetes(t, true)
		chain := &fakeHostname{err: errNoHostname}
		h, ec2Calls := newTestStandaloneHostname(coreconfig.NewMock(t), chain, "", errors.New("unable to fetch EC2 API"))

		_, err := h.Get(ctx)
		assert.ErrorIs(t, err, errNoHostname)
		assert.Equal(t, 1, *ec2Calls)

		// Nothing is cached on failure, so the next call tries again.
		_, err = h.Get(ctx)
		assert.ErrorIs(t, err, errNoHostname)
		assert.Equal(t, 2, chain.calls)
		assert.Equal(t, 2, *ec2Calls)
	})

	t.Run("original error when the instance ID is not a valid hostname", func(t *testing.T) {
		setKubernetes(t, true)
		chain := &fakeHostname{err: errNoHostname}
		h, _ := newTestStandaloneHostname(coreconfig.NewMock(t), chain, "not a hostname", nil)

		_, err := h.Get(ctx)
		assert.ErrorIs(t, err, errNoHostname)
	})
}

func TestSetStandaloneTraceHostname(t *testing.T) {
	ctx := context.Background()

	t.Run("standalone sets the resolved hostname", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"otel_standalone": true})
		h := &fakeHostname{data: hostnameinterface.Data{Hostname: "i-0123456789abcdef0", Provider: "aws"}}

		require.NoError(t, setStandaloneTraceHostname(ctx, cfg, h))
		assert.True(t, cfg.IsConfigured("hostname"))
		assert.Equal(t, "i-0123456789abcdef0", cfg.GetString("hostname"))
		assert.Equal(t, pkgconfigmodel.SourceAgentRuntime, cfg.GetSource("hostname"))
	})

	t.Run("standalone keeps a configured hostname", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"otel_standalone": true, "hostname": "my-host"})
		h := &fakeHostname{data: hostnameinterface.Data{Hostname: "i-0123456789abcdef0", Provider: "aws"}}

		require.NoError(t, setStandaloneTraceHostname(ctx, cfg, h))
		assert.Equal(t, "my-host", cfg.GetString("hostname"))
		assert.Zero(t, h.calls)
	})

	t.Run("standalone fails when no hostname can be resolved", func(t *testing.T) {
		cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"otel_standalone": true})
		h := &fakeHostname{err: errNoHostname}

		err := setStandaloneTraceHostname(ctx, cfg, h)
		assert.ErrorIs(t, err, errNoHostname)
		assert.False(t, cfg.IsConfigured("hostname"))
	})

	t.Run("connected mode leaves the hostname to the core agent", func(t *testing.T) {
		cfg := coreconfig.NewMock(t)
		h := &fakeHostname{data: hostnameinterface.Data{Hostname: "i-0123456789abcdef0", Provider: "aws"}}

		require.NoError(t, setStandaloneTraceHostname(ctx, cfg, h))
		assert.False(t, cfg.IsConfigured("hostname"))
		assert.Zero(t, h.calls)
	})
}
