// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package dogtelextensionimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	agentmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

// newTestExtension creates a dogtelExtension wired with test doubles.
func newTestExtension(t *testing.T, cfgOverrides map[string]interface{}, extCfg *Config) *dogtelExtension {
	t.Helper()
	if extCfg == nil {
		extCfg = createDefaultConfig().(*Config)
	}
	hostname, _ := hostnameinterface.NewMock("test-host")
	return &dogtelExtension{
		config:     extCfg,
		log:        logmock.New(t),
		coreConfig: configmock.NewMockWithOverrides(t, cfgOverrides),
		serializer: serializermock.NewMetricSerializer(t),
		hostname:   hostname,
		telemetry:  noopsimpl.GetCompatComponent(),
		ipc:        ipcmock.New(t),
		buildInfo:  component.BuildInfo{},
	}
}

// TestStart_ConnectedMode verifies that Start is a no-op when otel_standalone=false.
func TestStart_ConnectedMode(t *testing.T) {
	ext := newTestExtension(t, map[string]interface{}{"otel_standalone": false}, nil)
	err := ext.Start(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, ext.taggerServer)
	assert.Equal(t, 0, ext.taggerServerPort)
}

// TestStart_StandaloneMode_TaggerDisabled verifies that Start succeeds in standalone
// mode without starting the tagger server when it is disabled.
func TestStart_StandaloneMode_TaggerDisabled(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.EnableTaggerServer = false

	hostname, _ := hostnameinterface.NewMock("test-host")
	serializer := serializermock.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Return(nil)

	ext := &dogtelExtension{
		config:     cfg,
		log:        logmock.New(t),
		coreConfig: configmock.NewMockWithOverrides(t, map[string]interface{}{"otel_standalone": true}),
		serializer: serializer,
		hostname:   hostname,
		telemetry:  noopsimpl.GetCompatComponent(),
		ipc:        ipcmock.New(t),
		buildInfo:  component.BuildInfo{Version: "1.0", Command: "otel-agent"},
	}

	err := ext.Start(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, ext.taggerServer)
}

// TestShutdown_NoTaggerServer verifies Shutdown does not panic when no server is running.
func TestShutdown_NoTaggerServer(t *testing.T) {
	ext := newTestExtension(t, nil, nil)
	err := ext.Shutdown(context.Background())
	require.NoError(t, err)
}

// TestGetTaggerServerPort returns the stored port.
func TestGetTaggerServerPort(t *testing.T) {
	ext := newTestExtension(t, nil, nil)
	assert.Equal(t, 0, ext.GetTaggerServerPort())

	ext.taggerServerPort = 15555
	assert.Equal(t, 15555, ext.GetTaggerServerPort())
}

// TestSendLivenessMetric_Success verifies SendIterableSeries is called with a SerieSource.
func TestSendLivenessMetric_Success(t *testing.T) {
	hostname, _ := hostnameinterface.NewMock("my-host")

	serializer := serializermock.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.AnythingOfType("*metrics.IterableSeries")).Return(nil)

	ext := &dogtelExtension{
		log:        logmock.New(t),
		coreConfig: configmock.NewMockWithOverrides(t, nil),
		serializer: serializer,
		hostname:   hostname,
		buildInfo:  component.BuildInfo{Version: "7.0.0", Command: "otel-agent"},
	}

	err := ext.sendLivenessMetric(context.Background())
	require.NoError(t, err)
	serializer.AssertExpectations(t)
}

// TestSendLivenessMetric_SerializerError verifies errors from SendIterableSeries are propagated.
func TestSendLivenessMetric_SerializerError(t *testing.T) {
	hostname, _ := hostnameinterface.NewMock("my-host")
	wantErr := errors.New("serializer unavailable")

	serializer := serializermock.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Return(wantErr)

	ext := &dogtelExtension{
		log:        logmock.New(t),
		coreConfig: configmock.NewMockWithOverrides(t, nil),
		serializer: serializer,
		hostname:   hostname,
	}

	err := ext.sendLivenessMetric(context.Background())
	require.ErrorIs(t, err, wantErr)
}

// TestSendLivenessMetric_UsesHostname verifies the hostname is passed to the liveness serie.
func TestSendLivenessMetric_UsesHostname(t *testing.T) {
	hostname, _ := hostnameinterface.NewMock("expected-host")

	var captured []*agentmetrics.Serie
	serializer := serializermock.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		src := args.Get(0).(agentmetrics.SerieSource)
		// Consume all series while the goroutine is running.
		for src.MoveNext() {
			if s := src.Current(); s != nil {
				sc := *s
				captured = append(captured, &sc)
			}
		}
	}).Return(nil)

	ext := &dogtelExtension{
		log:        logmock.New(t),
		coreConfig: configmock.NewMockWithOverrides(t, nil),
		serializer: serializer,
		hostname:   hostname,
		buildInfo:  component.BuildInfo{},
	}

	err := ext.sendLivenessMetric(context.Background())
	require.NoError(t, err)
	require.Len(t, captured, 1)
	assert.Equal(t, "otel.dogtel_extension.running", captured[0].Name)
	assert.Equal(t, "expected-host", captured[0].Host)
	assert.Equal(t, 1.0, captured[0].Points[0].Value)
}

// TestIsSecretsNoop_WithNoopImpl verifies that the noop impl is detected.
func TestIsSecretsNoop_WithNoopImpl(t *testing.T) {
	var s secrets.Component = &secretnooptypes.SecretNoop{}
	assert.True(t, isSecretsNoop(s))
}

// TestIsSecretsNoop_WithNilSecrets verifies that a nil component returns false.
func TestIsSecretsNoop_WithNilSecrets(t *testing.T) {
	assert.False(t, isSecretsNoop(nil))
}

// TestStart_StandaloneMode_NoopSecretsWarning verifies that Start succeeds even
// when the noop secrets impl is injected in standalone mode (the warning is logged
// but does not prevent startup).
func TestStart_StandaloneMode_NoopSecretsWarning(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.EnableTaggerServer = false

	hostname, _ := hostnameinterface.NewMock("test-host")
	sz := serializermock.NewMetricSerializer(t)
	sz.On("SendIterableSeries", mock.Anything).Return(nil)

	ext := &dogtelExtension{
		config:     cfg,
		log:        logmock.New(t),
		coreConfig: configmock.NewMockWithOverrides(t, map[string]interface{}{"otel_standalone": true}),
		serializer: sz,
		hostname:   hostname,
		telemetry:  noopsimpl.GetCompatComponent(),
		ipc:        ipcmock.New(t),
		// Deliberately inject the noop impl to simulate a misconfiguration.
		secrets: &secretnooptypes.SecretNoop{},
	}

	// Start should succeed; the warning is logged but not fatal.
	err := ext.Start(context.Background(), nil)
	require.NoError(t, err)
}
